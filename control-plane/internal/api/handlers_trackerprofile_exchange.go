// Export and import of a tracker's site profile (ADR-037, #197).
//
// The exchange file is layer 2 (site profile) with an identification envelope — and that's it.
// The handlers read the profile via trackerprofile.Service, which never queries announce, passkey,
// api_url nor api_token; the export is the serialization of the Document TYPE, which has no field for
// any of them. There is no list of fields here to forget to filter.
//
// The imported file is UNTRUSTED content: the body goes through trackerprofile.ParseDocument
// (size, shape, types, version, family, ranges) before anything else.

package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
)

// importNotice accompanies EVERY preview, for the same reason as discoverNotice: the warning
// is from the engine, not the screen — any API client needs to know that the content came from
// outside and was not checked against this operator's site.
const importNotice = "This profile came from a file, not from your site: ids and names were not checked against the " +
	"catalog of your tracker. A wrong id publishes to the wrong category — check the preview of each " +
	"release, and run discovery for the site to confirm what belongs to it."

// readDocument reads the body (the file itself) with a size limit and validates it. In case of
// refusal it already responded to the client and returns ok=false.
func readDocument(w http.ResponseWriter, r *http.Request, d Deps) (trackerprofile.Document, map[trackerprofile.OverwriteKey]bool, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, trackerprofile.MaxDocumentBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, trackerprofile.CodeTooLarge,
				fmt.Sprintf("file exceeds %d KiB", trackerprofile.MaxDocumentBytes>>10))
		} else {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "unreadable body")
		}
		return trackerprofile.Document{}, nil, false
	}
	overwrite, err := trackerprofile.ParseOverwrite(r.URL.Query()["overwrite"])
	if err != nil {
		profileError(w, d, err)
		return trackerprofile.Document{}, nil, false
	}
	doc, err := trackerprofile.ParseDocument(raw)
	if err != nil {
		profileError(w, d, err)
		return trackerprofile.Document{}, nil, false
	}
	return doc, overwrite, true
}

// handleExportTrackerProfile returns the profile's exchange file. It is read-only (trackers:read):
// the content is already in the trackers listing, and what makes it exportable is precisely that it does not
// contain anything that the listing doesn't also show besides name and host.
func handleExportTrackerProfile(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		doc, err := d.Profiles.Export(r.Context(), user.ID, r.PathValue("id"))
		if err != nil {
			profileError(w, d, err)
			return
		}
		body, err := doc.Marshal()
		if err != nil {
			d.Logger.Error("serialize exported profile", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to generate file")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		// The file name comes from trackerprofile.Document.Filename: only [a-z0-9-].
		w.Header().Set("Content-Disposition", `attachment; filename="`+doc.Filename()+`"`)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

// descriptionOutcome is what the import did with the description section, in words for the trail.
var descriptionOutcome = map[string]string{
	trackerprofile.StatusNew:       "applied",
	trackerprofile.StatusReplaced:  "replaced on request",
	trackerprofile.StatusProtected: "kept the operator's",
	trackerprofile.StatusSame:      "already the same",
}

// imageHostOutcome is what the import did with the image_host section (#193), in words for the trail.
// Same statuses as description: the section is a unit, and the merge rule is the same.
var imageHostOutcome = map[string]string{
	trackerprofile.StatusNew:       "applied",
	trackerprofile.StatusReplaced:  "replaced on request",
	trackerprofile.StatusProtected: "kept the operator's",
	trackerprofile.StatusSame:      "already the same",
}

// previewResponse is the import preview. Nothing was saved.
type previewResponse struct {
	Plan   trackerprofile.ImportPlan `json:"plan"`
	Notice string                    `json:"notice"`
}

// handlePreviewProfileImport validates the file and returns what it would change. It never saves — so,
// like discovery, it stays out of the audit trail (skipAuditPath): the operator's action
// is to apply.
func handlePreviewProfileImport(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		doc, overwrite, ok := readDocument(w, r, d)
		if !ok {
			return
		}
		plan, err := d.Profiles.PreviewImport(r.Context(), user.ID, r.PathValue("id"), doc, overwrite)
		if err != nil {
			profileError(w, d, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, previewResponse{Plan: plan, Notice: importNotice})
	}
}

// handleImportTrackerProfile applies the file. It is the second explicit call of the safe mode:
// the preview (…/import/preview) never saves, this one does. The coexistence rule with what the
// operator adjusted manually runs here, on the server, over the locked state.
func handleImportTrackerProfile(d Deps) http.HandlerFunc {
	type response struct {
		Profile trackerprofile.Profile    `json:"profile"`
		Plan    trackerprofile.ImportPlan `json:"plan"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		doc, overwrite, ok := readDocument(w, r, d)
		if !ok {
			return
		}
		res, err := d.Profiles.Import(r.Context(), user.ID, r.PathValue("id"), doc, overwrite)
		if err != nil {
			profileError(w, d, err)
			return
		}
		// Nothing from the file goes to the trail (its site name is third-party text, and the
		// «» quotes are what delimit the tracker name): only the tracker, by the secure label, and the accounts.
		c := res.Plan.Counts
		action := fmt.Sprintf(
			"Imported a profile from a file into tracker «%s»: %d new, %d renamed, %d role(s) filled; "+
				"%d kept because they belong to the operator or were already confirmed by its site",
			trackerprofile.AuditLabel(res.TrackerName), c.New, c.Renamed, c.KindSet, c.Protected)
		// The description section (#192) and the image host section (#193) only enter the sentence when the
		// file brought them; in the data, their status ("" when not brought).
		var descStatus, hostStatus string
		if dp := res.Plan.Description; dp != nil {
			descStatus = dp.Status
			action += "; description layout: " + descriptionOutcome[dp.Status]
		}
		if ih := res.Plan.ImageHost; ih != nil {
			hostStatus = ih.Status
			action += "; image host: " + imageHostOutcome[ih.Status]
		}
		httpx.SetAudit(r.Context(), "tracker_profile_imported", map[string]any{"tracker": trackerprofile.AuditLabel(res.TrackerName),
			"new": c.New, "renamed": c.Renamed, "kind_set": c.KindSet, "protected": c.Protected,
			"description": descStatus, "image_host": hostStatus}, action)
		httpx.WriteJSON(w, http.StatusOK, response{Profile: res.Profile, Plan: res.Plan})
	}
}
