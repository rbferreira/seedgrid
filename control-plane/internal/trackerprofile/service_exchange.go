package trackerprofile

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Export, preview and application of the import (#197). The trackers table is only queried
// to check ownership and existence and read name and host — the announce, the passkey, the api_url and the
// token never enter a query from here, and therefore cannot reach the file.

// loadFor reads what the export and preview need: tracker name and host (to
// identify the site) and the profile. A tracker without a profile line is read as an empty profile.
func (s *Service) loadFor(ctx context.Context, userID, trackerID string) (Site, Profile, error) {
	if !validUUID(trackerID) {
		return Site{}, Profile{}, ErrNotFound
	}
	var name, host string
	var family *string
	var version *int
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT t.name, t.host, p.family, p.schema_version, p.taxonomy
		  FROM trackers t
		  LEFT JOIN tracker_profiles p ON p.tracker_id = t.id AND p.deleted_at IS NULL
		 WHERE t.id = $1 AND t.user_id = $2 AND t.deleted_at IS NULL`,
		trackerID, userID).Scan(&name, &host, &family, &version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Site{}, Profile{}, ErrNotFound
	}
	if err != nil {
		return Site{}, Profile{}, err
	}
	site := targetSite(name, host)
	if family == nil || version == nil {
		return site, Empty(), nil
	}
	p, err := decodeProfile(*family, *version, raw)
	return site, p, err
}

// targetSite is the tracker's identification as the file shows it: the name goes through siteLabel
// (never the operator's free text, which may contain a credential) and the host is the DNS name.
func targetSite(name, host string) Site {
	h := publicHost(host)
	return Site{Name: siteLabel(name, h), Host: h}
}

// Export returns the exchange file for an operator's tracker profile. The generated file
// goes through the SAME import validation before leaving: the export never delivers a file that the
// import itself would reject — for example, a profile with two terms of the same name (case-insensitive and
// accent-insensitive), which the publishing panel wouldn't be able to distinguish. In this case the operator receives the
// reason, instead of a file that whoever imports it cannot use.
func (s *Service) Export(ctx context.Context, userID, trackerID string) (Document, error) {
	site, p, err := s.loadFor(ctx, userID, trackerID)
	if err != nil {
		return Document{}, err
	}
	doc := NewDocument(site.Name, site.Host, p)
	if _, err := doc.validated(); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return Document{}, invalidCode(CodeNotExportable, ve.Path,
				"this tracker's profile cannot be exported as is — fix it and export again: %s", ve.Msg)
		}
		return Document{}, err
	}
	return doc, nil
}

// PreviewImport says what the file would change in the tracker's profile. It only reads: it doesn't write anything
// nor does it create the profile line.
func (s *Service) PreviewImport(ctx context.Context, userID, trackerID string, doc Document, overwrite map[OverwriteKey]bool) (ImportPlan, error) {
	target, cur, err := s.loadFor(ctx, userID, trackerID)
	if err != nil {
		return ImportPlan{}, err
	}
	if err := doc.CheckTarget(cur); err != nil {
		return ImportPlan{}, err
	}
	return PlanImport(cur, doc, target, overwrite)
}

// ImportResult is the outcome of an applied import: the profile as it ended up and the plan as it was
// EXECUTED (calculated on the locked state, which may differ from what the screen previewed).
type ImportResult struct {
	TrackerName string
	Profile     Profile
	Plan        ImportPlan
}

// Import applies the file (already validated by ParseDocument). The plan is calculated INSIDE the
// transaction, on the locked line — the same guarantee as discovery's Apply: the rule that
// protects what the operator adjusted by hand runs on what is in the database now.
func (s *Service) Import(ctx context.Context, userID, trackerID string, doc Document, overwrite map[OverwriteKey]bool) (ImportResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	name, cur, err := lock(ctx, tx, userID, trackerID)
	if err != nil {
		return ImportResult{}, err
	}
	if err := doc.CheckTarget(cur); err != nil {
		return ImportResult{}, err
	}
	var host string
	if err := tx.QueryRow(ctx, `SELECT host FROM trackers WHERE id = $1`, trackerID).Scan(&host); err != nil {
		return ImportResult{}, err
	}
	plan, err := PlanImport(cur, doc, targetSite(name, host), overwrite)
	if err != nil {
		return ImportResult{}, err
	}
	if plan.Changes() {
		// The three sections come from the SAME plan, calculated on the locked line: the taxonomy, the
		// description and the image_host are written together, or none at all.
		cur.Taxonomy = ApplyImport(cur.Taxonomy, plan)
		cur.Description = ApplyImportDescription(cur.Description, plan)
		cur.ImageHost = ApplyImportImageHost(cur.ImageHost, plan)
		if err := save(ctx, tx, trackerID, cur); err != nil {
			return ImportResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{TrackerName: name, Profile: cur, Plan: plan}, nil
}
