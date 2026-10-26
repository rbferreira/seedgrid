package trackerprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/description"
	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
)

// ErrNotFound: tracker does not exist or was removed.
var ErrNotFound = errors.New("tracker not found")

// Service operates on the tracker_profiles table (migration 00043). The trackers table is only
// queried to check ownership and existence and read the name — never the announce or the token.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the profile service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Result is the outcome of a write: the profile as it ended up, the tracker name (for the
// Audit trail to say WHICH tracker) and, in discovery application, the report.
type Result struct {
	TrackerName string
	Profile     Profile
	Applied     []Applied
	Skipped     []Skipped
}

// validUUID blocks anything that doesn't have UUID shape before reaching Postgres: a malformed
// URL {id} would be a cast error (500), and the right thing is "not found".
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
		default:
			return false
		}
	}
	return true
}

// docDescription is the RESERVED key, at the root of the JSONB document, for the description section (#192).
//
// The column is named `taxonomy` for historical reasons (migration 00043), but it stores the entire
// profile document: the taxonomy axes at the root — the format every existing row has — and,
// alongside them, a key for each new section. This allowed adding the section WITHOUT a migration and
// without rewriting any row: the old document is, byte for byte, a new document without the section.
// An axis is never named "description" (the set of axes is closed, see KnownAxis), so the
// key cannot collide.
const docDescription = "description"

// docImageHost is the RESERVED key, at the root of the JSONB document, for the image_host section (#193).
// It follows the same pattern as docDescription: an optional and additive section, without migration, that coexists
// with the taxonomy axes at the root of the document. An axis is never named "image_host" (the set
// of axes is closed, see KnownAxis), so the key cannot collide.
const docImageHost = "image_host"

// decodeProfile reads the JSONB document. The axes become the taxonomy; the reserved keys become
// the description and image_host sections. A wrong type in any of them is an error (an unreadable
// document cannot be read as "empty" and then overwritten).
func decodeProfile(family string, version int, raw []byte) (Profile, error) {
	p := Profile{Family: family, SchemaVersion: version}
	var tax Taxonomy
	if len(raw) > 0 {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(raw, &doc); err != nil {
			return Profile{}, fmt.Errorf("profile taxonomy unreadable: %w", err)
		}
		tax = make(Taxonomy, len(doc))
		for key, val := range doc {
			if key == docDescription {
				if bytes.Equal(bytes.TrimSpace(val), []byte("null")) {
					continue
				}
				var c description.Config
				if err := json.Unmarshal(val, &c); err != nil {
					return Profile{}, fmt.Errorf("profile description section unreadable: %w", err)
				}
				n := c.Normalized()
				p.Description = &n
				continue
			}
			if key == docImageHost {
				if bytes.Equal(bytes.TrimSpace(val), []byte("null")) {
					continue
				}
				var s imagehost.Spec
				if err := json.Unmarshal(val, &s); err != nil {
					return Profile{}, fmt.Errorf("profile image_host section unreadable: %w", err)
				}
				p.ImageHost = &s
				continue
			}
			var terms []Term
			if err := json.Unmarshal(val, &terms); err != nil {
				return Profile{}, fmt.Errorf("profile taxonomy unreadable: %w", err)
			}
			tax[key] = terms
		}
	}
	p.Taxonomy = normalize(tax)
	return p, nil
}

// encodeProfile is the inverse of decodeProfile: the axes at the root and, if present, the
// description and image_host sections.
func encodeProfile(p Profile) ([]byte, error) {
	doc := make(map[string]any, len(p.Taxonomy)+2)
	for axis, terms := range p.Taxonomy {
		doc[axis] = terms
	}
	if p.Description != nil {
		doc[docDescription] = p.Description
	}
	if p.ImageHost != nil {
		doc[docImageHost] = p.ImageHost
	}
	return json.Marshal(doc)
}

// ImageHostSpec returns only the image_host section of the profile (nil = no custom host), for the
// screenshot chain (torrent.TrackerProfileProvider, ADR-040). ErrNotFound same as Get.
func (s *Service) ImageHostSpec(ctx context.Context, trackerID string) (*imagehost.Spec, error) {
	p, err := s.Get(ctx, trackerID)
	if err != nil {
		return nil, err
	}
	return p.ImageHost, nil
}

// Get returns the profile for a tracker. A tracker without a profile row is not an error:
// it returns the empty profile (all axes present, no terms).
func (s *Service) Get(ctx context.Context, trackerID string) (Profile, error) {
	if !validUUID(trackerID) {
		return Profile{}, ErrNotFound
	}
	var family *string
	var version *int
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT p.family, p.schema_version, p.taxonomy
		  FROM trackers t
		  LEFT JOIN tracker_profiles p ON p.tracker_id = t.id AND p.deleted_at IS NULL
		 WHERE t.id = $1 AND t.deleted_at IS NULL`,
		trackerID).Scan(&family, &version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	if family == nil || version == nil {
		return Empty(), nil
	}
	return decodeProfile(*family, *version, raw)
}

// ForTrackers returns the profiles of all active trackers in the cluster, by tracker
// id. Those without a profile row simply do not appear in the map.
func (s *Service) ForTrackers(ctx context.Context) (map[string]Profile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.tracker_id::text, p.family, p.schema_version, p.taxonomy
		  FROM tracker_profiles p
		  JOIN trackers t ON t.id = p.tracker_id
		 WHERE t.deleted_at IS NULL AND p.deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Profile{}
	for rows.Next() {
		var id, family string
		var version int
		var raw []byte
		if err := rows.Scan(&id, &family, &version, &raw); err != nil {
			return nil, err
		}
		p, err := decodeProfile(family, version, raw)
		if err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// lock ensures the tracker's profile row exists and locks it for the transaction (FOR UPDATE):
// editing and discovery application are read-modify-write, and two at the same time
// without a lock would lose one of them.
func lock(ctx context.Context, tx pgx.Tx, trackerID string) (string, Profile, error) {
	if !validUUID(trackerID) {
		return "", Profile{}, ErrNotFound
	}
	var name string
	err := tx.QueryRow(ctx,
		`SELECT name FROM trackers WHERE id = $1 AND deleted_at IS NULL`,
		trackerID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Profile{}, ErrNotFound
	}
	if err != nil {
		return "", Profile{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tracker_profiles (tracker_id) VALUES ($1)
		ON CONFLICT (tracker_id) WHERE deleted_at IS NULL DO NOTHING`, trackerID); err != nil {
		return "", Profile{}, err
	}
	var family string
	var version int
	var raw []byte
	if err := tx.QueryRow(ctx, `
		SELECT family, schema_version, taxonomy FROM tracker_profiles
		 WHERE tracker_id = $1 AND deleted_at IS NULL FOR UPDATE`,
		trackerID).Scan(&family, &version, &raw); err != nil {
		return "", Profile{}, err
	}
	p, err := decodeProfile(family, version, raw)
	return name, p, err
}

// save writes the entire profile document (taxonomy + sections). Whoever doesn't touch a section
// passes it on as it was read: writing only the taxonomy would delete the description.
func save(ctx context.Context, tx pgx.Tx, trackerID string, p Profile) error {
	raw, err := encodeProfile(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE tracker_profiles SET taxonomy = $2::jsonb, updated_at = now()
		 WHERE tracker_id = $1 AND deleted_at IS NULL`, trackerID, string(raw))
	return err
}

// Edit applies a manual taxonomy edit (already validated by ValidateInput). The axes
// present in `in` are replaced; the missing ones stay as they are. The origin of each term
// is decided here, by comparing with what is stored (see ApplyEdit).
func (s *Service) Edit(ctx context.Context, trackerID string, in map[string][]TermInput) (Result, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	name, cur, err := lock(ctx, tx, trackerID)
	if err != nil {
		return Result{}, err
	}
	cur.Taxonomy = ApplyEdit(cur.Taxonomy, in)
	if err := save(ctx, tx, trackerID, cur); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{TrackerName: name, Profile: cur}, nil
}

// SetDescription replaces the profile's description section (#192). nil deletes it: the tracker reverts
// to the default. The Config goes through CleanDescription HERE, in the service — not just in the handler — because
// it is a write point and validation cannot depend on the caller. The taxonomy is passed through
// as is.
func (s *Service) SetDescription(ctx context.Context, trackerID string, cfg *description.Config) (Result, error) {
	var clean *description.Config
	if cfg != nil {
		c, err := CleanDescription(*cfg)
		if err != nil {
			return Result{}, err
		}
		clean = &c
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	name, cur, err := lock(ctx, tx, trackerID)
	if err != nil {
		return Result{}, err
	}
	cur.Description = clean
	if err := save(ctx, tx, trackerID, cur); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{TrackerName: name, Profile: cur}, nil
}

// SetImageHost replaces the profile's image_host section (#193). nil deletes it: the tracker starts
// using only the global imgbb in the screenshot chain. The Spec goes through CleanImageHost HERE,
// in the service — not just in the handler — because it is a write point and validation cannot
// depend on the caller. The taxonomy and other sections are passed through as is.
func (s *Service) SetImageHost(ctx context.Context, trackerID string, spec *imagehost.Spec) (Result, error) {
	var clean *imagehost.Spec
	if spec != nil {
		c, err := CleanImageHost(*spec)
		if err != nil {
			return Result{}, err
		}
		clean = &c
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	name, cur, err := lock(ctx, tx, trackerID)
	if err != nil {
		return Result{}, err
	}
	cur.ImageHost = clean
	if err := save(ctx, tx, trackerID, cur); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{TrackerName: name, Profile: cur}, nil
}

// Apply applies accepted items from a discovery proposal (already validated by
// ValidateApplyItems) using the Merge rule: completes what's missing, never overwrites the
// manual. Merge runs INSIDE the transaction, on the locked state — not on what the
// Portal saw when the proposal was generated, which may have changed since then.
func (s *Service) Apply(ctx context.Context, trackerID string, items []ApplyItem) (Result, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	name, cur, err := lock(ctx, tx, trackerID)
	if err != nil {
		return Result{}, err
	}
	next, applied, skipped, err := Merge(cur.Taxonomy, items)
	if err != nil {
		return Result{}, err
	}
	cur.Taxonomy = next
	if len(applied) > 0 {
		if err := save(ctx, tx, trackerID, cur); err != nil {
			return Result{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{TrackerName: name, Profile: cur, Applied: applied, Skipped: skipped}, nil
}

// Forget soft-deletes the profile of a removed tracker. Idempotent.
func (s *Service) Forget(ctx context.Context, trackerID string) error {
	if !validUUID(trackerID) {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE tracker_profiles SET deleted_at = now(), updated_at = now()
		 WHERE tracker_id = $1 AND deleted_at IS NULL`, trackerID)
	return err
}
