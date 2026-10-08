// Package igdb is a minimal IGDB API client (backlog #17), used to enrich
// the publishing of GAMES/ROMs on a UNIT3D tracker — analogous to the tmdb package (which serves
// movies/series). IGDB is from Twitch: auth is OAuth "client credentials" (machine-to-
// machine) — client_id + client_secret → app access token (no user login/cookie),
// self-renewed. Enrichment is optional: without credentials (Enabled()==false) the
// game publishing proceeds without the igdb_id. Read-only (search).
package igdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrDisabled: client without credentials. ErrNotFound: no results.
var (
	ErrDisabled = errors.New("IGDB not configured")
	ErrNotFound = errors.New("no results on IGDB")
)

const (
	defaultTokenBase = "https://id.twitch.tv" //nolint:gosec // G101: it is a URL, not a credential
	defaultAPIBase   = "https://api.igdb.com"
	// t_cover_big = 264×374; enough for the tracker's entry [img].
	coverBase = "https://images.igdb.com/igdb/image/upload/t_cover_big/"
)

// Game is a candidate from the IGDB catalog (useful subset for the tracker's entry).
type Game struct {
	IGDBID    int      `json:"igdb_id"`
	Name      string   `json:"name"`
	Year      int      `json:"year"`
	Summary   string   `json:"summary"`
	CoverURL  string   `json:"cover_url"`
	Genres    []string `json:"genres"`
	Platforms []string `json:"platforms"`
}

// Client talks to IGDB using a Twitch app access token (client credentials),
// cached until close to expiration.
//
// Credentials come from providers read ON EVERY use (ADR-029): they live in the catalog (ADR-023),
// editable in the Portal, so changing them takes effect without a redeploy. The cached token stores which
// credential pair it was obtained from — changing the credential in the panel invalidates the cache immediately.
type Client struct {
	idFn      func() string
	secretFn  func() string
	tokenBase string
	apiBase   string
	http      *http.Client

	mu         sync.Mutex
	token      string
	expires    time.Time
	tokenCreds string // credentials that generated the cached token (invalidates upon change)
}

// NewClient creates the client over credential providers (e.g., settings.Store).
// Providers that return "" produce a disabled client at that moment.
func NewClient(idFn, secretFn func() string) *Client {
	if idFn == nil {
		idFn = func() string { return "" }
	}
	if secretFn == nil {
		secretFn = func() string { return "" }
	}
	return &Client{
		idFn:      idFn,
		secretFn:  secretFn,
		tokenBase: defaultTokenBase,
		apiBase:   defaultAPIBase,
		http:      &http.Client{Timeout: 12 * time.Second},
	}
}

// clientID/clientSecret return the current credential, already trimmed.
func (c *Client) clientID() string     { return strings.TrimSpace(c.idFn()) }
func (c *Client) clientSecret() string { return strings.TrimSpace(c.secretFn()) }

// WithBases swaps the base URLs (for testing). Returns the Client itself.
func (c *Client) WithBases(tokenBase, apiBase string) *Client {
	if tokenBase != "" {
		c.tokenBase = strings.TrimRight(tokenBase, "/")
	}
	if apiBase != "" {
		c.apiBase = strings.TrimRight(apiBase, "/")
	}
	return c
}

// Enabled reports whether credentials are configured.
func (c *Client) Enabled() bool { return c.clientID() != "" && c.clientSecret() != "" }

// Search looks for games by title and returns the candidates (the operator picks the right one in the
// panel — IGDB search sometimes prioritizes mods/hacks). Returns ErrDisabled without creds.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Game, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrNotFound
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	tok, err := c.appToken(ctx)
	if err != nil {
		return nil, err
	}

	// apicalypse: double quotes break the query → removed from the search term.
	safe := strings.ReplaceAll(query, `"`, "")
	body := fmt.Sprintf(
		`search "%s"; fields name,first_release_date,summary,genres.name,platforms.name,cover.image_id; limit %d;`,
		safe, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/v4/games", bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", c.clientID())
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IGDB: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("IGDB: status %d on search", resp.StatusCode)
	}

	var raw []struct {
		ID               int    `json:"id"`
		Name             string `json:"name"`
		FirstReleaseDate int64  `json:"first_release_date"` // epoch seconds
		Summary          string `json:"summary"`
		Genres           []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Platforms []struct {
			Name string `json:"name"`
		} `json:"platforms"`
		Cover struct {
			ImageID string `json:"image_id"`
		} `json:"cover"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("IGDB: invalid response: %w", err)
	}
	if len(raw) == 0 {
		return nil, ErrNotFound
	}

	out := make([]Game, 0, len(raw))
	for _, g := range raw {
		game := Game{IGDBID: g.ID, Name: g.Name, Summary: g.Summary}
		if g.FirstReleaseDate > 0 {
			game.Year = time.Unix(g.FirstReleaseDate, 0).UTC().Year()
		}
		if g.Cover.ImageID != "" {
			game.CoverURL = coverBase + g.Cover.ImageID + ".jpg"
		}
		for _, x := range g.Genres {
			if x.Name != "" {
				game.Genres = append(game.Genres, x.Name)
			}
		}
		for _, x := range g.Platforms {
			if x.Name != "" {
				game.Platforms = append(game.Platforms, x.Name)
			}
		}
		out = append(out, game)
	}
	return out, nil
}

// Details fetches a game by ID (subset of what the UNIT3D browse needs: cover,
// genres, year). IGDB sometimes doesn't return igdb_id in the tracker browse; in that case
// the handler falls back to Search(name). Returns ErrNotFound if the id does not exist.
func (c *Client) Details(ctx context.Context, id int) (Game, error) {
	if !c.Enabled() {
		return Game{}, ErrDisabled
	}
	if id <= 0 {
		return Game{}, ErrNotFound
	}
	tok, err := c.appToken(ctx)
	if err != nil {
		return Game{}, err
	}
	body := fmt.Sprintf(
		`where id = %d; fields name,first_release_date,summary,genres.name,platforms.name,cover.image_id; limit 1;`,
		id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/v4/games", bytes.NewBufferString(body))
	if err != nil {
		return Game{}, err
	}
	req.Header.Set("Client-ID", c.clientID())
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Game{}, fmt.Errorf("IGDB: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Game{}, fmt.Errorf("IGDB: status %d on details", resp.StatusCode)
	}
	var raw []struct {
		ID               int    `json:"id"`
		Name             string `json:"name"`
		FirstReleaseDate int64  `json:"first_release_date"`
		Summary          string `json:"summary"`
		Genres           []struct {
			Name string `json:"name"`
		} `json:"genres"`
		Platforms []struct {
			Name string `json:"name"`
		} `json:"platforms"`
		Cover struct {
			ImageID string `json:"image_id"`
		} `json:"cover"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Game{}, fmt.Errorf("IGDB: invalid response: %w", err)
	}
	if len(raw) == 0 {
		return Game{}, ErrNotFound
	}
	g := raw[0]
	out := Game{IGDBID: g.ID, Name: g.Name, Summary: g.Summary}
	if g.FirstReleaseDate > 0 {
		out.Year = time.Unix(g.FirstReleaseDate, 0).UTC().Year()
	}
	if g.Cover.ImageID != "" {
		out.CoverURL = coverBase + g.Cover.ImageID + ".jpg"
	}
	for _, x := range g.Genres {
		if x.Name != "" {
			out.Genres = append(out.Genres, x.Name)
		}
	}
	for _, x := range g.Platforms {
		if x.Name != "" {
			out.Platforms = append(out.Platforms, x.Name)
		}
	}
	return out, nil
}

// appToken returns a valid app access token, reusing the cached one as long as it's not
// close to expiring; otherwise it performs the client-credentials grant on Twitch.
func (c *Client) appToken(ctx context.Context) (string, error) {
	id, secret := c.clientID(), c.clientSecret()
	creds := id + "\x00" + secret
	c.mu.Lock()
	defer c.mu.Unlock()
	// Reuses the token only if still valid AND obtained from the CURRENT credentials: if the operator
	// changed the key in the Portal (ADR-029), the old token is from another account → discards it.
	if c.token != "" && c.tokenCreds == creds && time.Now().Before(c.expires) {
		return c.token, nil
	}
	q := url.Values{}
	q.Set("client_id", id)
	q.Set("client_secret", secret)
	q.Set("grant_type", "client_credentials")
	u := c.tokenBase + "/oauth2/token?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("IGDB token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("IGDB token: status %d", resp.StatusCode)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"` // seconds
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", fmt.Errorf("IGDB token: invalid response: %w", err)
	}
	if t.AccessToken == "" {
		return "", fmt.Errorf("IGDB token: empty")
	}
	c.token = t.AccessToken
	c.tokenCreds = creds // tag of which credential this token came from (ADR-029)
	// Renews with a 60s margin (or in ~50min if expires_in is not provided).
	ttl := time.Duration(t.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 50 * time.Minute
	}
	c.expires = time.Now().Add(ttl - 60*time.Second)
	return c.token, nil
}
