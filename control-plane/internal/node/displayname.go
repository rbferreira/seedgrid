package node

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Node Alias (#145): a name that lives ONLY within SeedGrid. The OS hostname is assigned
// by infra (and the operator prefers it numbered, like seedgrid-node-01); the alias is what the
// Portal shows when present. Renaming does not touch the Node nor require restarting the Agent.

// MaxDisplayNameLen is the maximum length of the alias, in characters (not bytes).
const MaxDisplayNameLen = 64

// displayNameIndex is the partial unique index from 00042; the name is what Postgres
// returns in PgError.ConstraintName on a violation.
const displayNameIndex = "uk_nodes_display_name_per_cluster"

// Nickname errors.
var (
	// ErrInvalidDisplayName wraps the reason (empty of meaning, too long,
	// non-printable character) — the error message is already readable for the operator.
	ErrInvalidDisplayName = errors.New("invalid display name")
	// ErrDisplayNameTaken: another Node (not removed) already claims this name — either as
	// an alias or as a hostname.
	ErrDisplayNameTaken = errors.New("a Node with this name already exists")
)

// NormalizeDisplayName applies the alias rules: leading/trailing spaces are removed, runs of
// space/tab/newline become a single space (two names that only differ in invisible
// space are the same name for the operator), and the result must fit within
// MaxDisplayNameLen and contain only printable characters (no control, no
// zero-width — which would fabricate visually identical "duplicates").
//
// "" is valid and means NO alias: the Portal falls back to showing the hostname.
func NormalizeDisplayName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", nil
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: text is not valid UTF-8", ErrInvalidDisplayName)
	}
	if n := utf8.RuneCountInString(name); n > MaxDisplayNameLen {
		return "", fmt.Errorf("%w: %d characters (maximum %d)", ErrInvalidDisplayName, n, MaxDisplayNameLen)
	}
	for _, r := range name {
		// IsPrint rejects control, format (zero-width) and non-ASCII spaces
		// — all "special" spaces have already been collapsed by strings.Fields.
		if !unicode.IsPrint(r) {
			return "", fmt.Errorf("%w: contains non-printable character", ErrInvalidDisplayName)
		}
	}
	return name, nil
}

// nameTaken says whether ANOTHER non-removed Node already claims name, as an alias or as a
// hostname (case-insensitive). exceptNodeID is the Node itself (empty = none):
// renaming to the name it already has is not a conflict. Comparing with hostnames too is
// what ensures that two Nodes never display the same label.
func (s *Service) nameTaken(ctx context.Context, name, exceptNodeID string) (bool, error) {
	var taken bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM nodes
		     WHERE deleted_at IS NULL
		       AND ($2 = '' OR id::text <> $2)
		       AND (lower(hostname) = lower($1) OR lower(display_name) = lower($1)))`,
		name, exceptNodeID).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("check Node name: %w", err)
	}
	return taken, nil
}

// RenameResult describes an alias change, for the audit trail (ADR-025).
// Empty Before/After = "no alias".
type RenameResult struct {
	Hostname string
	Before   string
	After    string
}

// SetDisplayName saves (or, with empty raw, removes) a Node's alias. It only touches the
// Control Plane database — the Node is not notified nor does it need to restart anything.
func (s *Service) SetDisplayName(ctx context.Context, nodeID, raw string) (RenameResult, error) {
	name, err := NormalizeDisplayName(raw)
	if err != nil {
		return RenameResult{}, err
	}

	var res RenameResult
	var before *string
	err = s.pool.QueryRow(ctx,
		`SELECT hostname, display_name FROM nodes WHERE id = $1 AND deleted_at IS NULL`,
		nodeID).Scan(&res.Hostname, &before)
	if errors.Is(err, pgx.ErrNoRows) {
		return RenameResult{}, ErrNodeNotFound
	}
	if err != nil {
		return RenameResult{}, fmt.Errorf("query node: %w", err)
	}
	if before != nil {
		res.Before = *before
	}
	res.After = name

	if name != "" {
		taken, err := s.nameTaken(ctx, name, nodeID)
		if err != nil {
			return RenameResult{}, err
		}
		if taken {
			return RenameResult{}, ErrDisplayNameTaken
		}
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE nodes SET display_name = NULLIF($2, ''), updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL`, nodeID, name)
	if err != nil {
		if isDisplayNameViolation(err) { // race: another request won between the check and the UPDATE
			return RenameResult{}, ErrDisplayNameTaken
		}
		return RenameResult{}, fmt.Errorf("write alias: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return RenameResult{}, ErrNodeNotFound // removed between the SELECT and the UPDATE
	}
	return res, nil
}

// isDisplayNameViolation recognizes the alias unique index violation (23505).
func isDisplayNameViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == displayNameIndex
}

// registrationName decides the alias that the registration actually saves. The name comes from the join
// token (already validated upon issuance), but the world changes between issuance and use: another Node may
// have taken the name, or the Agent may have sent its own value in the legacy path.
// In any doubt the Node ENTERS without an alias — failing registration because of a name
// would be worse than the problem; the operator renames it later via the Portal. The second return
// says that a name was requested and discarded, for the registration event to log.
func (s *Service) registrationName(ctx context.Context, hostname, raw string) (name string, dropped bool) {
	name, err := NormalizeDisplayName(raw)
	if err != nil {
		return "", true
	}
	if name == "" {
		return "", false
	}
	// The Node itself (same hostname, re-registration) does not count as a conflict.
	var self string
	_ = s.pool.QueryRow(ctx,
		`SELECT id::text FROM nodes WHERE cluster_id = $1 AND hostname = $2 AND deleted_at IS NULL`,
		s.defaultClusterID, hostname).Scan(&self)
	taken, err := s.nameTaken(ctx, name, self)
	if err != nil || taken {
		return "", true
	}
	return name, false
}
