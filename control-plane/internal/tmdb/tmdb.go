// Package tmdb is a minimal client for the TMDB v3 API (ADR-020), used to
// resolve TMDB/IMDB IDs (and TVDB, for series) from the title + year
// parsed from a release. It's an optional enrichment: without a key (Enabled()==false)
// the publish proceeds without the IDs. Read-only (search + external_ids).
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrDisabled: client without an API key. ErrNotFound: no results.
var (
	ErrDisabled = errors.New("TMDB not configured")
	ErrNotFound = errors.New("no results on TMDB")
)

const (
	defaultBase = "https://api.themoviedb.org"
	imageBase   = "https://image.tmdb.org/t/p/w500"
	langEN      = "en-US"
)

// Result groups the identification fields of a work.
type Result struct {
	TMDBID    int    `json:"tmdb_id"`
	IMDBID    string `json:"imdb"` // e.g.: tt1375666 ("" if absent)
	TVDBID    int    `json:"tvdb"` // 0 if absent (series only)
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Overview  string `json:"overview"`
	PosterURL string `json:"poster_url"`
	IsTV      bool   `json:"is_tv"`
}

// Details is the per-work metadata subset used on the Explore screen (#12) to
// enrich the catalog: PT translated title + original title, poster, synopsis,
// genres and original language. Unlike Result (search identification), Details
// comes by already known ID (UNIT3D returns tmdb_id on browse).
type Details struct {
	TMDBID       int      `json:"tmdb_id"`
	Title        string   `json:"title"`         // translated title (pt-BR); falls back to the original when there is no translation
	OriginalName string   `json:"original_name"` // original title (e.g., in English/Japanese)
	Overview     string   `json:"overview"`      // synopsis in pt-BR (falls back to en-US if empty)
	PosterURL    string   `json:"poster_url"`
	Year         int      `json:"year"`
	Genres       []string `json:"genres"`
	OrigLanguage string   `json:"original_language"` // ISO-639-1 code (e.g., "en", "ja")
	IsTV         bool     `json:"is_tv"`
}

// Client talks to the TMDB API using the v3 key (api_key in the query).
//
// The key comes from a provider read ON EVERY use (ADR-029): the key lives in the
// tunables catalog (ADR-023) and is editable in the Portal, so changing or clearing it
// works without a redeploy — the client never caches a fixed value from boot.
type Client struct {
	keyFn func() string
	base  string
	http  *http.Client
}

// NewClient creates the client over a key provider (e.g., settings.Store). A provider that
// returns "" produces a disabled client at that moment.
func NewClient(keyFn func() string) *Client {
	if keyFn == nil {
		keyFn = func() string { return "" }
	}
	return &Client{
		keyFn: keyFn,
		base:  defaultBase,
		http:  &http.Client{Timeout: 10 * time.Second},
	}
}

// key returns the current key, already trimmed of spaces.
func (c *Client) key() string { return strings.TrimSpace(c.keyFn()) }

// WithBase swaps the base URL (for testing). Returns the Client itself.
func (c *Client) WithBase(base string) *Client {
	if base != "" {
		c.base = strings.TrimRight(base, "/")
	}
	return c
}

// Enabled reports whether there is a configured key.
func (c *Client) Enabled() bool { return c.key() != "" }

// Lookup searches for the work by title + year (isTV chooses movie vs series) and resolves
// external IDs (IMDB, and TVDB for series). Returns ErrDisabled/ErrNotFound as applicable.
func (c *Client) Lookup(ctx context.Context, title string, year int, isTV bool, lang string) (Result, error) {
	if !c.Enabled() {
		return Result{}, ErrDisabled
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Result{}, ErrNotFound
	}

	kind := "movie"
	yearParam := "year"
	if isTV {
		kind = "tv"
		yearParam = "first_air_date_year"
	}

	q := url.Values{}
	q.Set("api_key", c.key())
	q.Set("query", title)
	q.Set("include_adult", "false")
	q.Set("language", apiLanguage(lang))
	if year > 0 {
		q.Set(yearParam, strconv.Itoa(year))
	}

	var search struct {
		Results []struct {
			ID           int    `json:"id"`
			Title        string `json:"title"` // movie
			Name         string `json:"name"`  // tv
			Overview     string `json:"overview"`
			PosterPath   string `json:"poster_path"`
			ReleaseDate  string `json:"release_date"`   // movie (YYYY-MM-DD)
			FirstAirDate string `json:"first_air_date"` // tv
		} `json:"results"`
	}
	if err := c.getJSON(ctx, "/3/search/"+kind, q, &search); err != nil {
		return Result{}, err
	}
	if len(search.Results) == 0 {
		return Result{}, ErrNotFound
	}
	r := search.Results[0]

	out := Result{TMDBID: r.ID, IsTV: isTV, Overview: r.Overview}
	if isTV {
		out.Title = r.Name
		out.Year = yearFromDate(r.FirstAirDate)
	} else {
		out.Title = r.Title
		out.Year = yearFromDate(r.ReleaseDate)
	}
	if r.PosterPath != "" {
		out.PosterURL = imageBase + r.PosterPath
	}

	// Not every work has a translated synopsis on TMDB: if pt-BR comes back empty, it fetches the
	// original (English) in the details to avoid leaving the description blank. Best-effort.
	if out.Overview == "" && apiLanguage(lang) != langEN {
		var det struct {
			Overview string `json:"overview"`
		}
		dq := url.Values{}
		dq.Set("api_key", c.key())
		dq.Set("language", langEN)
		if err := c.getJSON(ctx, fmt.Sprintf("/3/%s/%d", kind, r.ID), dq, &det); err == nil {
			out.Overview = det.Overview
		}
	}

	// External IDs (IMDB always; TVDB only for series). Best-effort: failure here does not
	// invalidate the match (IDs remain empty and the operator completes it).
	var ext struct {
		IMDBID string `json:"imdb_id"`
		TVDBID int    `json:"tvdb_id"`
	}
	eq := url.Values{}
	eq.Set("api_key", c.key())
	if err := c.getJSON(ctx, fmt.Sprintf("/3/%s/%d/external_ids", kind, r.ID), eq, &ext); err == nil {
		out.IMDBID = ext.IMDBID
		if isTV {
			out.TVDBID = ext.TVDBID
		}
	}
	return out, nil
}

// Details fetches by ID (movie or tv) the display metadata used on the Explore screen:
// translated title, original title, poster, synopsis, genres and original language. Falls
// back to the en-US overview when pt-BR has no translation — the UI shouldn't be left
// blank just because the work hasn't been translated yet.
func (c *Client) Details(ctx context.Context, id int, isTV bool, lang string) (Details, error) {
	if !c.Enabled() {
		return Details{}, ErrDisabled
	}
	if id <= 0 {
		return Details{}, ErrNotFound
	}
	kind := "movie"
	if isTV {
		kind = "tv"
	}
	q := url.Values{}
	q.Set("api_key", c.key())
	q.Set("language", apiLanguage(lang))

	var det struct {
		ID               int    `json:"id"`
		Title            string `json:"title"`          // movie
		OriginalTitle    string `json:"original_title"` // movie
		Name             string `json:"name"`           // tv
		OriginalName     string `json:"original_name"`  // tv
		Overview         string `json:"overview"`
		PosterPath       string `json:"poster_path"`
		ReleaseDate      string `json:"release_date"`   // movie
		FirstAirDate     string `json:"first_air_date"` // tv
		OriginalLanguage string `json:"original_language"`
		Genres           []struct {
			Name string `json:"name"`
		} `json:"genres"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("/3/%s/%d", kind, id), q, &det); err != nil {
		return Details{}, err
	}
	if det.ID == 0 {
		return Details{}, ErrNotFound
	}

	out := Details{TMDBID: det.ID, IsTV: isTV, Overview: det.Overview, OrigLanguage: det.OriginalLanguage}
	if isTV {
		out.Title = det.Name
		out.OriginalName = det.OriginalName
		out.Year = yearFromDate(det.FirstAirDate)
	} else {
		out.Title = det.Title
		out.OriginalName = det.OriginalTitle
		out.Year = yearFromDate(det.ReleaseDate)
	}
	if det.PosterPath != "" {
		out.PosterURL = imageBase + det.PosterPath
	}
	for _, g := range det.Genres {
		if g.Name != "" {
			out.Genres = append(out.Genres, g.Name)
		}
	}

	// Translated title missing: falls back to the original (the UI shows "Original Title" in
	// highlight instead of leaving it blank). Best-effort — just a local fallback.
	if strings.TrimSpace(out.Title) == "" {
		out.Title = out.OriginalName
	}

	// Synopsis without pt-BR translation: fetches the original (en-US) to avoid leaving it blank.
	if strings.TrimSpace(out.Overview) == "" && apiLanguage(lang) != langEN {
		var en struct {
			Overview string `json:"overview"`
		}
		eq := url.Values{}
		eq.Set("api_key", c.key())
		eq.Set("language", langEN)
		if err := c.getJSON(ctx, fmt.Sprintf("/3/%s/%d", kind, id), eq, &en); err == nil {
			out.Overview = en.Overview
		}
	}
	return out, nil
}

// getJSON performs a GET and decodes the body into v. Status errors become errors.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, v any) error {
	u := c.base + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("TMDB: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("TMDB: status %d em %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// yearFromDate extracts the year from a "YYYY-MM-DD" date; 0 if empty/invalid.
func yearFromDate(d string) int {
	if len(d) < 4 {
		return 0
	}
	y, err := strconv.Atoi(d[:4])
	if err != nil {
		return 0
	}
	return y
}

// apiLanguage maps a content language of the site profile (#74: "en", "pt-BR") to TMDB's
// language parameter. Anything else is English.
func apiLanguage(lang string) string {
	if lang == "pt-BR" {
		return "pt-BR"
	}
	return langEN
}
