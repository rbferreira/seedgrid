// Package tracker manages the operator's tracker credentials (ADR-019/ADR-020):
// name + announce URL pairs (with passkey) used to publish content to private
// trackers. The announce is a SECRET — stored in plain text in Postgres, but NEVER
// returned by the API (the listing only exposes name + host). The CP injects the announce only
// into the publish command to the Agent. "Public" (no tracker, DHT/PeX) is not a
// row here — it's the absence of a tracker on publish.
//
// ADR-020: a tracker can be a UNIT3D site (e.g. tracker.example). In this case it gains
// integration fields — source (.torrent label), api_url (site base) and
// api_token (SECRET). Like the announce, the api_token is never returned in the API: the listing
// only exposes has_api_token (boolean).
package tracker

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound: tracker does not exist (or belongs to another operator).
	ErrNotFound = errors.New("tracker not found")
	// ErrDuplicateName: a tracker with this name already exists for the operator.
	ErrDuplicateName = errors.New("a tracker with this name already exists")
	// ErrInvalidAnnounce: empty or malformed announce URL.
	ErrInvalidAnnounce = errors.New("invalid announce URL")
	// ErrInvalidAPIURL: malformed api_url (when provided).
	ErrInvalidAPIURL = errors.New("invalid api_url")
)

// Tracker is the public view of a tracker (WITHOUT the announce/passkey or the api_token).
// Source and APIURL are not secrets and are exposed; HasAPIToken only indicates the presence
// of the API token (UNIT3D integration ready), without revealing it. HasImageHostKey indicates
// the presence of the operator's key for the tracker's image host (#193 / ADR-040);
// ImageHostKeyHost is the DNS name for which this key was saved (the listing exposes it
// so the Portal can warn when the profile's host changed and the key got stuck).
type Tracker struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Host             string    `json:"host"`
	Source           string    `json:"source"`
	APIURL           string    `json:"api_url"`
	HasAPIToken      bool      `json:"has_api_token"`
	HasImageHostKey  bool      `json:"has_image_host_key"`
	ImageHostKeyHost string    `json:"image_host_key_host"`
	IsDefault        bool      `json:"is_default"`
	CreatedAt        time.Time `json:"created_at"`
}

// Input gathers the write fields (Create/Update). Source/APIURL/APIToken are
// optional (UNIT3D integration). In Update, an empty APIToken means "keep the
// current one" (secret is never returned to be resent); to clear it, delete the tracker.
type Input struct {
	Name        string
	Announce    string
	Source      string
	APIURL      string
	APIToken    string
	MakeDefault bool
}

// Secret gathers the internal fields used on publish (ADR-019/ADR-020). It is NEVER
// serialized to the API — only the CP uses it when dispatching the command/POST.
type Secret struct {
	Announce string
	Source   string
	APIURL   string
	APIToken string
}

// Service operates on the trackers table.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the tracker service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// parseAnnounce validates the announce URL and extracts the host (for display). Accepts
// http(s) and udp (schemes used by BitTorrent trackers).
func parseAnnounce(announce string) (host string, err error) {
	announce = strings.TrimSpace(announce)
	if announce == "" {
		return "", ErrInvalidAnnounce
	}
	u, perr := url.Parse(announce)
	if perr != nil || u.Host == "" {
		return "", ErrInvalidAnnounce
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "udp":
	default:
		return "", ErrInvalidAnnounce
	}
	return u.Host, nil
}

// normalizeAPIURL validates the api_url (when provided) and returns it without a trailing slash.
// Empty is valid (tracker without UNIT3D integration) and returns "".
func normalizeAPIURL(apiURL string) (string, error) {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return "", nil
	}
	u, perr := url.Parse(apiURL)
	if perr != nil || u.Host == "" {
		return "", ErrInvalidAPIURL
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", ErrInvalidAPIURL
	}
	return strings.TrimRight(apiURL, "/"), nil
}

// isUniqueViolation reports whether err is a Postgres unique violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// List returns the operator's trackers (without the announce or the api_token), default first.
func (s *Service) List(ctx context.Context, userID string) ([]Tracker, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, host,
		       COALESCE(source, ''), COALESCE(api_url, ''),
		       (api_token IS NOT NULL AND api_token <> '') AS has_api_token,
		       (image_host_key IS NOT NULL AND image_host_key <> '') AS has_image_host_key,
		       COALESCE(image_host_key_host, '') AS image_host_key_host,
		       is_default, created_at
		  FROM trackers
		 WHERE user_id = $1 AND deleted_at IS NULL
		 ORDER BY is_default DESC, name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tracker{}
	for rows.Next() {
		var t Tracker
		if err := rows.Scan(&t.ID, &t.Name, &t.Host, &t.Source, &t.APIURL, &t.HasAPIToken, &t.HasImageHostKey, &t.ImageHostKeyHost, &t.IsDefault, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// validateCreate normalizes and validates the Create fields (announce required),
// returning the announce host and the normalized api_url.
func validateCreate(in Input) (name, host, apiURL string, err error) {
	name = strings.TrimSpace(in.Name)
	if name == "" {
		return "", "", "", ErrInvalidAnnounce
	}
	host, err = parseAnnounce(in.Announce)
	if err != nil {
		return "", "", "", err
	}
	apiURL, err = normalizeAPIURL(in.APIURL)
	if err != nil {
		return "", "", "", err
	}
	return name, host, apiURL, nil
}

// Create registers a tracker. If MakeDefault, it unsets the previous operator's
// default in the same transaction (there is only one default per operator).
func (s *Service) Create(ctx context.Context, userID string, in Input) (Tracker, error) {
	name, host, apiURL, err := validateCreate(in)
	if err != nil {
		return Tracker{}, err
	}
	source := strings.TrimSpace(in.Source)
	apiToken := strings.TrimSpace(in.APIToken)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Tracker{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if in.MakeDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE trackers SET is_default = false, updated_at = now()
			  WHERE user_id = $1 AND is_default AND deleted_at IS NULL`, userID); err != nil {
			return Tracker{}, err
		}
	}
	var t Tracker
	err = tx.QueryRow(ctx, `
		INSERT INTO trackers (user_id, name, announce, host, source, api_url, api_token, is_default)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8)
		RETURNING id::text, name, host,
		          COALESCE(source, ''), COALESCE(api_url, ''),
		          (api_token IS NOT NULL AND api_token <> ''),
		          (image_host_key IS NOT NULL AND image_host_key <> ''),
		          COALESCE(image_host_key_host, ''),
		          is_default, created_at`,
		userID, name, strings.TrimSpace(in.Announce), host, source, apiURL, apiToken, in.MakeDefault,
	).Scan(&t.ID, &t.Name, &t.Host, &t.Source, &t.APIURL, &t.HasAPIToken, &t.HasImageHostKey, &t.ImageHostKeyHost, &t.IsDefault, &t.CreatedAt)
	if isUniqueViolation(err) {
		return Tracker{}, ErrDuplicateName
	}
	if err != nil {
		return Tracker{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Tracker{}, err
	}
	return t, nil
}

// Update changes the name/source/api_url of an operator's tracker. The SECRETS
// (announce and api_token) are only changed when provided filled — empty = keep the
// current one, since neither is returned in the API to be resent. This allows editing
// the UNIT3D integration without retyping the passkey. MakeDefault is ignored (use SetDefault).
func (s *Service) Update(ctx context.Context, userID, id string, in Input) (Tracker, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Tracker{}, ErrInvalidAnnounce
	}
	// empty announce = keep; if provided, validates and recomputes the host.
	announce := strings.TrimSpace(in.Announce)
	var host string
	if announce != "" {
		var herr error
		if host, herr = parseAnnounce(announce); herr != nil {
			return Tracker{}, herr
		}
	}
	apiURL, err := normalizeAPIURL(in.APIURL)
	if err != nil {
		return Tracker{}, err
	}
	source := strings.TrimSpace(in.Source)
	apiToken := strings.TrimSpace(in.APIToken)

	var t Tracker
	err = s.pool.QueryRow(ctx, `
		UPDATE trackers SET
		    name       = $3,
		    announce   = CASE WHEN $4 = '' THEN announce ELSE $4 END,
		    host       = CASE WHEN $4 = '' THEN host ELSE $5 END,
		    source     = NULLIF($6, ''),
		    api_url    = NULLIF($7, ''),
		    api_token  = CASE WHEN $8 = '' THEN api_token ELSE $8 END,
		    updated_at = now()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING id::text, name, host,
		          COALESCE(source, ''), COALESCE(api_url, ''),
		          (api_token IS NOT NULL AND api_token <> ''),
		          (image_host_key IS NOT NULL AND image_host_key <> ''),
		          COALESCE(image_host_key_host, ''),
		          is_default, created_at`,
		id, userID, name, announce, host, source, apiURL, apiToken,
	).Scan(&t.ID, &t.Name, &t.Host, &t.Source, &t.APIURL, &t.HasAPIToken, &t.HasImageHostKey, &t.ImageHostKeyHost, &t.IsDefault, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tracker{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Tracker{}, ErrDuplicateName
	}
	if err != nil {
		return Tracker{}, err
	}
	return t, nil
}

// Delete soft-deletes an operator's tracker.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE trackers SET deleted_at = now(), is_default = false, updated_at = now()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDefault sets a tracker as the operator's default (unsets the previous one).
func (s *Service) SetDefault(ctx context.Context, userID, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`UPDATE trackers SET is_default = false, updated_at = now()
		  WHERE user_id = $1 AND is_default AND deleted_at IS NULL`, userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE trackers SET is_default = true, updated_at = now()
		  WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// SetImageHostKey saves the operator's key for the image host (#193 / ADR-040).
// The key is tied to the host it was registered for: if the profile's upload_url changes
// host, the old key no longer works (the upload chain skips it). Empty key is
// normalized to NULL (but the handler rejects it before). Returns the tracker name for
// Audit (ADR-025); ErrNotFound if the tracker doesn't exist or belongs to another operator.
func (s *Service) SetImageHostKey(ctx context.Context, userID, id, key, host string) (string, error) {
	if !validUUID(id) {
		return "", ErrNotFound
	}
	var name string
	err := s.pool.QueryRow(ctx, `
		UPDATE trackers SET
		    image_host_key      = NULLIF($3, ''),
		    image_host_key_host = NULLIF($4, ''),
		    updated_at          = now()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		 RETURNING name`, id, userID, key, host).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return name, nil
}

// ClearImageHostKey deletes the image host key (#193 / ADR-040). Idempotent —
// deleting what doesn't exist is also a success (the handler returns 200). Returns the
// tracker name for Audit; ErrNotFound if the tracker doesn't exist or belongs to another
// operator.
func (s *Service) ClearImageHostKey(ctx context.Context, userID, id string) (string, error) {
	if !validUUID(id) {
		return "", ErrNotFound
	}
	var name string
	err := s.pool.QueryRow(ctx, `
		UPDATE trackers SET
		    image_host_key      = NULL,
		    image_host_key_host = NULL,
		    updated_at          = now()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		 RETURNING name`, id, userID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return name, nil
}

// ImageHostKeyForChain returns the key and the host it was registered for, for use
// in the screenshot upload chain (#193 / ADR-040). If there is no key, it returns
// ("", "", nil). ErrNotFound if the tracker doesn't exist or belongs to another operator.
func (s *Service) ImageHostKeyForChain(ctx context.Context, userID, id string) (key, host string, err error) {
	if !validUUID(id) {
		return "", "", ErrNotFound
	}
	var k, h *string
	err = s.pool.QueryRow(ctx, `
		SELECT image_host_key, image_host_key_host FROM trackers
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID).
		Scan(&k, &h)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if k != nil {
		key = *k
	}
	if h != nil {
		host = *h
	}
	return key, host, nil
}

// ForPublish returns the publish secrets of an operator's tracker (announce +
// UNIT3D integration). Internal use for publishing — NEVER exposed in the API. Returns
// ErrNotFound if it doesn't exist.
func (s *Service) ForPublish(ctx context.Context, userID, id string) (Secret, error) {
	var sec Secret
	var source, apiURL, apiToken *string
	err := s.pool.QueryRow(ctx,
		`SELECT announce, source, api_url, api_token FROM trackers
		  WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID).
		Scan(&sec.Announce, &source, &apiURL, &apiToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return Secret{}, ErrNotFound
	}
	if err != nil {
		return Secret{}, err
	}
	if source != nil {
		sec.Source = *source
	}
	if apiURL != nil {
		sec.APIURL = *apiURL
	}
	if apiToken != nil {
		sec.APIToken = *apiToken
	}
	return sec, nil
}

// ObserveDLSlotLimit records the per-account DL slot limit observed from the
// tracker's own message (#21 — UNIT3D error 164 "You may at most download N
// torrents"). The Agent reads the Engine msg and reports it; the CP stores it in the default tracker
// (the passkey/account is shared across the cluster's Nodes). limit<=0 is ignored.
func (s *Service) ObserveDLSlotLimit(ctx context.Context, limit int) error {
	if limit <= 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE trackers SET dl_slot_limit = $1, dl_slot_limit_seen_at = now(), updated_at = now()
		  WHERE is_default AND deleted_at IS NULL`, limit)
	return err
}

// ObservedDLSlotLimit returns the last observed L on the default tracker, as long as the
// observation is more recent than ttl. (0, false) when there is no valid observation
// (never seen or too old) — in this case the controller falls back to the config fallback.
func (s *Service) ObservedDLSlotLimit(ctx context.Context, ttl time.Duration) (int, bool) {
	var limit int
	err := s.pool.QueryRow(ctx,
		`SELECT dl_slot_limit FROM trackers
		  WHERE is_default AND deleted_at IS NULL
		    AND dl_slot_limit IS NOT NULL
		    AND dl_slot_limit_seen_at > now() - make_interval(secs => $1)
		  ORDER BY dl_slot_limit_seen_at DESC LIMIT 1`, ttl.Seconds()).Scan(&limit)
	if err != nil {
		return 0, false
	}
	return limit, true
}

// validUUID blocks what doesn't have UUID format before reaching Postgres: a malformed URL {id}
// would be a cast error (500), and the correct response is "not found". Same rule as
// trackerprofile.
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
