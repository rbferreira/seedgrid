// Handlers for a tracker's site profile (ADR-037, #190/#196): taxonomy editing,
// discovery from the tracker's own catalog, and proposal application.
//
// The profile is layer 2 of ADR-037 (site data, exportable). What the operator stores
// in `trackers` — announce, api_token — is layer 3 and never passes through here: handlers
// read api_url/api_token via tracker.Service.ForPublish only to call the site, and do not
// pass them to any response or the profile.

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rbferreira/seedgrid/control-plane/internal/description"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
	"github.com/rbferreira/seedgrid/control-plane/internal/unit3d"
)

// maxProfileBody limits the body of profile requests. Three full axes (256 terms
// of 64 characters) fit comfortably in 64 KiB; the cap exists for hostile bodies.
const maxProfileBody = 1 << 20

// discoverNotice is the warning that accompanies EVERY discovery. It goes in the response
// (not just the screen) because the limitation is in the engine, not the UI: any API client
// must know the result is incomplete by definition (ADR-037, Limitations).
const discoverNotice = "Discovery only sees what has already been posted to the catalog: category, type, or resolution " +
	"what no one has used yet does not appear, and the catalog is scanned from newest to oldest. " +
	"The discovered profile is incomplete by definition — whatever is missing is added manually."

// trackerView is the tracker as returned by the API: the public view of layer 3 (without
// announce or token) plus the site profile (layer 2).
type trackerView struct {
	tracker.Tracker
	Profile trackerprofile.Profile `json:"profile"`
	// DescriptionDefault (#192) is the description composition that applies when the profile lacks the
	// description section — and, by listing ALL blocks in their original order, the catalog of what the
	// editor offers to toggle on, off, and reorder. It comes from the server so the Portal doesn't
	// duplicate the default: what the profile has of its own is in profile.description.
	DescriptionDefault description.Config `json:"description_default"`
}

// profilesOf reads the operator's profiles. Failure becomes an empty profile + log: the tracker list
// cannot disappear because of an auxiliary table.
func profilesOf(ctx context.Context, d Deps, userID string) map[string]trackerprofile.Profile {
	if d.Profiles == nil {
		return nil
	}
	m, err := d.Profiles.ForTrackers(ctx, userID)
	if err != nil {
		d.Logger.Error("read tracker profiles", "error", err)
		return nil
	}
	return m
}

// viewOf joins the tracker to its profile; without a profile row, the empty profile.
func viewOf(t tracker.Tracker, profiles map[string]trackerprofile.Profile) trackerView {
	p, ok := profiles[t.ID]
	if !ok {
		p = trackerprofile.Empty()
	}
	return trackerView{Tracker: t, Profile: p, DescriptionDefault: description.DefaultConfig()}
}

// profileError maps profile errors to HTTP responses.
func profileError(w http.ResponseWriter, d Deps, err error) {
	var ve *trackerprofile.ValidationError
	switch {
	case errors.Is(err, trackerprofile.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "tracker_not_found", "tracker not found")
	case errors.As(err, &ve):
		httpx.WriteError(w, validationStatus(ve), ve.ErrorCode(), ve.Error())
	default:
		d.Logger.Error("tracker profile", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to process tracker profile")
	}
}

// validationStatus picks the refusal status: 413 for what exceeds the size, 409 for a
// file of a family the target tracker isn't, 400 for the rest.
func validationStatus(ve *trackerprofile.ValidationError) int {
	switch ve.ErrorCode() {
	case trackerprofile.CodeTooLarge:
		return http.StatusRequestEntityTooLarge
	case trackerprofile.CodeIncompatibleFamily, trackerprofile.CodeNotExportable:
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

// decodeProfileBody reads the body with a size cap. Unknown fields are refused
// (DecodeJSON): a profile with a key no one understands is an error, not something to ignore.
func decodeProfileBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBody)
	if err := httpx.DecodeJSON(r, dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
		return false
	}
	return true
}

// handleEditTrackerProfile edits the taxonomy manually. Sent axes REPLACE the
// current ones; missing ones stay as they are. The server decides the source of each term.
func handleEditTrackerProfile(d Deps) http.HandlerFunc {
	type request struct {
		Taxonomy map[string][]trackerprofile.TermInput `json:"taxonomy"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var req request
		if !decodeProfileBody(w, r, &req) {
			return
		}
		if len(req.Taxonomy) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_profile", "taxonomy: specify at least one axis")
			return
		}
		clean, err := trackerprofile.ValidateInput(req.Taxonomy)
		if err != nil {
			profileError(w, d, err)
			return
		}
		res, err := d.Profiles.Edit(r.Context(), user.ID, r.PathValue("id"), clean)
		if err != nil {
			profileError(w, d, err)
			return
		}
		httpx.SetAudit(r.Context(), "tracker_profile_edited", map[string]any{"tracker": res.TrackerName}, "Edited the profile of tracker «"+res.TrackerName+"»")
		httpx.WriteJSON(w, http.StatusOK, res.Profile)
	}
}

// handleSetTrackerDescription saves the profile's description section (#192): which blocks make up
// the upload description, in what order, and the MediaInfo tag. The body is the Config; what is missing
// (missing list, empty tag) is the default, and a `[]` list is "no blocks". Validation runs in the
// service — unknown id, repeated id, and tag outside the closed list are 400 invalid_profile with
// the field path. The taxonomy is untouched.
func handleSetTrackerDescription(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var cfg description.Config
		if !decodeProfileBody(w, r, &cfg) {
			return
		}
		res, err := d.Profiles.SetDescription(r.Context(), user.ID, r.PathValue("id"), &cfg)
		if err != nil {
			profileError(w, d, err)
			return
		}
		tracker := trackerprofile.AuditLabel(res.TrackerName)
		httpx.SetAudit(r.Context(), "tracker_description_edited", map[string]any{"tracker": tracker}, "Edited the description layout of tracker «"+tracker+"»")
		httpx.WriteJSON(w, http.StatusOK, res.Profile)
	}
}

// handleResetTrackerDescription deletes the description section: the tracker reverts to the default
// composition (the one the Portal always assembled). Idempotent — deleting what doesn't exist is also 200.
func handleResetTrackerDescription(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		res, err := d.Profiles.SetDescription(r.Context(), user.ID, r.PathValue("id"), nil)
		if err != nil {
			profileError(w, d, err)
			return
		}
		tracker := trackerprofile.AuditLabel(res.TrackerName)
		httpx.SetAudit(r.Context(), "tracker_description_reset", map[string]any{"tracker": tracker}, "Reset the description layout of tracker «"+tracker+"» to its default")
		httpx.WriteJSON(w, http.StatusOK, res.Profile)
	}
}

// handleSetTrackerImageHost saves the profile's image_host section (#193): the tracker's own
// image host. The body is the Spec; validation runs in the service — invalid upload_url, forbidden header
// and other ADR-040 rules are 400 invalid_profile with the field path.
func handleSetTrackerImageHost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var spec imagehost.Spec
		if !decodeProfileBody(w, r, &spec) {
			return
		}
		res, err := d.Profiles.SetImageHost(r.Context(), user.ID, r.PathValue("id"), &spec)
		if err != nil {
			profileError(w, d, err)
			return
		}
		tracker := trackerprofile.AuditLabel(res.TrackerName)
		httpx.SetAudit(r.Context(), "tracker_image_host_edited", map[string]any{"tracker": tracker}, "Edited the image host of tracker «"+tracker+"»")
		httpx.WriteJSON(w, http.StatusOK, res.Profile)
	}
}

// handleResetTrackerImageHost deletes the image_host section: the tracker starts using only the global
// imgbb in the screenshot chain. Idempotent — deleting what doesn't exist is also 200.
func handleResetTrackerImageHost(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		res, err := d.Profiles.SetImageHost(r.Context(), user.ID, r.PathValue("id"), nil)
		if err != nil {
			profileError(w, d, err)
			return
		}
		tracker := trackerprofile.AuditLabel(res.TrackerName)
		httpx.SetAudit(r.Context(), "tracker_image_host_removed", map[string]any{"tracker": tracker}, "Removed the image host of tracker «"+tracker+"»")
		httpx.WriteJSON(w, http.StatusOK, res.Profile)
	}
}

// handleSetTrackerImageHostKey saves the operator's key for the image host (#193).
// The key is bound to the current upload_url host: if the profile changes host, the old
// key no longer works. Empty key is 400; profile without image_host is 409.
func handleSetTrackerImageHostKey(d Deps) http.HandlerFunc {
	type request struct {
		Key string `json:"key"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		id := r.PathValue("id")
		var req request
		if !decodeProfileBody(w, r, &req) {
			return
		}
		key := strings.TrimSpace(req.Key)
		if key == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "key: key cannot be empty")
			return
		}
		// Check if the profile has the image_host section.
		profile, err := d.Profiles.Get(r.Context(), user.ID, id)
		if err != nil {
			profileError(w, d, err)
			return
		}
		if profile.ImageHost == nil {
			httpx.WriteError(w, http.StatusConflict, "profile_without_image_host",
				"tracker has no image host configured in the profile; configure the image_host section before registering the key")
			return
		}
		host := profile.ImageHost.Host()
		name, err := d.Trackers.SetImageHostKey(r.Context(), user.ID, id, key, host)
		if err != nil {
			trackerError(w, d, err)
			return
		}
		tracker := trackerprofile.AuditLabel(name)
		httpx.SetAudit(r.Context(), "tracker_image_host_key_set", map[string]any{"host": host, "tracker": tracker},
			"Saved the key for image host «"+host+"» on tracker «"+tracker+"»")
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleClearTrackerImageHostKey deletes the image host key. Idempotent.
func handleClearTrackerImageHostKey(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		id := r.PathValue("id")
		name, err := d.Trackers.ClearImageHostKey(r.Context(), user.ID, id)
		if err != nil {
			trackerError(w, d, err)
			return
		}
		tracker := trackerprofile.AuditLabel(name)
		httpx.SetAudit(r.Context(), "tracker_image_host_key_removed", map[string]any{"tracker": tracker}, "Removed the image host key of tracker «"+tracker+"»")
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// discoverRequest: the only parameter is how many pages to scan; empty = the default.
type discoverRequest struct {
	Pages int `json:"pages"`
}

// discoverResponse is the discovery proposal. Nothing here has been saved.
type discoverResponse struct {
	Family         string                                   `json:"family"`
	PagesRequested int                                      `json:"pages_requested"`
	PagesScanned   int                                      `json:"pages_scanned"`
	ItemsScanned   int                                      `json:"items_scanned"`
	StoppedReason  string                                   `json:"stopped_reason"`
	Proposal       map[string][]trackerprofile.ProposalItem `json:"proposal"`
	Rejected       int                                      `json:"rejected"`
	Warnings       []string                                 `json:"warnings"`
	Notice         string                                   `json:"notice"`
}

// buildDiscoverResponse joins the scan to the current profile and builds the proposal. Pure
// function, separated from the handler so the rule is testable without network or database.
func buildDiscoverResponse(current trackerprofile.Taxonomy, disc unit3d.Discovery, pagesRequested int) discoverResponse {
	observed := make(map[string][]trackerprofile.Observed, len(trackerprofile.Axes))
	for _, axis := range trackerprofile.Axes {
		for _, o := range disc.Axes[axis] {
			observed[axis] = append(observed[axis], trackerprofile.Observed{ID: o.ID, Name: o.Name, Seen: o.Seen})
		}
	}
	prop := trackerprofile.Propose(current, observed)
	warnings := prop.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return discoverResponse{
		Family:         trackerprofile.FamilyUNIT3D,
		PagesRequested: pagesRequested,
		PagesScanned:   disc.PagesScanned,
		ItemsScanned:   disc.ItemsScanned,
		StoppedReason:  string(disc.Stopped),
		Proposal:       prop.Items,
		Rejected:       prop.Rejected,
		Warnings:       warnings,
		Notice:         discoverNotice,
	}
}

// clampPages applies the default and the cap to the requested number of pages.
func clampPages(n int) int {
	if n <= 0 {
		return unit3d.DefaultDiscoverPages
	}
	return min(n, unit3d.MaxDiscoverPages)
}

// handleDiscoverTaxonomy scans N pages of the tracker's own catalog, with the operator's own
// token, and returns a taxonomy PROPOSAL. It never saves: applying is another
// call (apply-taxonomy), after the operator reviews it. Since it doesn't change state, it also doesn't
// go to the Audit trail (skipAuditPath) — the operator's action is the apply.
func handleDiscoverTaxonomy(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var req discoverRequest
		r.Body = http.MaxBytesReader(w, r.Body, maxProfileBody)
		// Empty body is valid (uses the default); present and malformed body is not.
		if err := httpx.DecodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if req.Pages < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "pages cannot be negative")
			return
		}

		current, err := d.Profiles.Get(r.Context(), user.ID, r.PathValue("id"))
		if err != nil {
			profileError(w, d, err)
			return
		}
		if current.Family != trackerprofile.FamilyUNIT3D {
			httpx.WriteError(w, http.StatusBadRequest, "unsupported_family", "discovery is only supported for the unit3d family")
			return
		}
		sec, err := d.Trackers.ForPublish(r.Context(), user.ID, r.PathValue("id"))
		if err != nil {
			trackerError(w, d, err)
			return
		}
		if strings.TrimSpace(sec.APIURL) == "" || strings.TrimSpace(sec.APIToken) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "tracker_without_api", "this tracker has no API integration (api_url + token)")
			return
		}

		pages := clampPages(req.Pages)
		disc := unit3d.DiscoverTaxonomy(r.Context(), sec.APIURL, sec.APIToken, unit3d.DiscoverOptions{MaxPages: pages})
		if disc.Err != nil {
			d.Logger.Warn("discover tracker taxonomy", "tracker", sec.Source,
				"pages_scanned", disc.PagesScanned, "stopped", string(disc.Stopped), "error", disc.Err)
		}
		// Failed before fetching any page: there is no proposal to show. With at least
		// one page, the partial is valid — and stopped_reason says it stopped before the end.
		if disc.PagesScanned == 0 && disc.Err != nil {
			httpx.WriteError(w, http.StatusBadGateway, "discover_failed", disc.Err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, buildDiscoverResponse(current.Taxonomy, disc, pages))
	}
}

// handleApplyTaxonomy applies the items the operator accepted from the proposal. The rule for
// coexisting with what was manually adjusted runs here, on the server, on the locked
// state (see trackerprofile.Merge) — the Portal might have an old picture.
func handleApplyTaxonomy(d Deps) http.HandlerFunc {
	type request struct {
		Items []trackerprofile.ApplyItem `json:"items"`
	}
	type response struct {
		Profile trackerprofile.Profile   `json:"profile"`
		Applied []trackerprofile.Applied `json:"applied"`
		Skipped []trackerprofile.Skipped `json:"skipped"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var req request
		if !decodeProfileBody(w, r, &req) {
			return
		}
		items, err := trackerprofile.ValidateApplyItems(req.Items)
		if err != nil {
			profileError(w, d, err)
			return
		}
		res, err := d.Profiles.Apply(r.Context(), user.ID, r.PathValue("id"), items)
		if err != nil {
			profileError(w, d, err)
			return
		}
		added, renamed := 0, 0
		for _, a := range res.Applied {
			if a.Action == trackerprofile.ActionAdded {
				added++
			} else {
				renamed++
			}
		}
		httpx.SetAudit(r.Context(), "tracker_taxonomy_applied", map[string]any{"tracker": res.TrackerName, "added": added, "renamed": renamed}, fmt.Sprintf(
			"Applied the discovered taxonomy to tracker «%s»: %d new, %d renamed",
			res.TrackerName, added, renamed))
		out := response{Profile: res.Profile, Applied: res.Applied, Skipped: res.Skipped}
		if out.Applied == nil {
			out.Applied = []trackerprofile.Applied{}
		}
		if out.Skipped == nil {
			out.Skipped = []trackerprofile.Skipped{}
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}
