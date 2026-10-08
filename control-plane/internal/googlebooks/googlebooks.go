// Package googlebooks is a minimal client for the Google Books Volumes API, used as the
// SECOND book/audiobook metadata provider in the Explore screen catalog (#110).
//
// It exists because Open Library — which is the first provider, open and keyless — does not
// index much of a Brazilian tracker's collection: children's audiobooks in PT-BR (Brothers Grimm,
// Monica's Gang), recent releases, and RPG zines. Measured in Aug/2026: 4 out of 14 books
// and 0 out of 3 audiobooks with covers. Google Books covers PT-BR and new releases much better.
//
// The key is technically optional but REQUIRED in practice: without it the API returns 429
// (Too Many Requests) after very few searches — a single catalog page already
// exceeds the IP's anonymous quota (measured in Aug/2026, not estimated). Without a key the client
// remains enabled and simply confirms nothing, which maintains the silent failure behavior of
// enrich: the book keeps what Open Library found, or has no cover.
// Read-only.
package googlebooks

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

// ErrNotFound: no plausible volume for the search.
var ErrNotFound = errors.New("no results on Google Books")

const defaultBase = "https://www.googleapis.com"

// Book is the subset of volumeInfo that the UI uses — purposefully identical to
// openlibrary.Book, so enrich can treat both providers the same way.
type Book struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Author   string   `json:"author"` // first author (+ " et al." when there are more)
	Year     int      `json:"year"`
	CoverURL string   `json:"cover_url"`
	Subjects []string `json:"subjects"` // volume `categories`, instead of "genres"
	Language string   `json:"language"` // ISO 639-1 ("pt", "en")
}

// Client talks to Google Books. The key comes from a provider read ON EVERY use
// (ADR-029): it lives in the settings catalog, editable in the Portal with hot-reload.
type Client struct {
	keyFn func() string
	base  string
	http  *http.Client
}

// NewClient creates the client. keyFn nil (or returning "") = anonymous use, which in practice
// hits 429 almost immediately — see the package comment.
func NewClient(keyFn func() string) *Client {
	if keyFn == nil {
		keyFn = func() string { return "" }
	}
	return &Client{
		keyFn: keyFn,
		base:  defaultBase,
		http:  &http.Client{Timeout: 12 * time.Second},
	}
}

// WithBase swaps the base URL (for tests). Returns the Client itself.
func (c *Client) WithBase(base string) *Client {
	if base != "" {
		c.base = strings.TrimRight(base, "/")
	}
	return c
}

// Enabled is always true: the API accepts anonymous calls (even if the quota is quickly exceeded).
// Kept so enrich can treat all providers through the same interface.
func (c *Client) Enabled() bool { return true }

/*
Search finds the most likely volume for title+author.

It uses the API qualifiers (`intitle:`/`inauthor:`) instead of free search: without them
Google Books matches the term in any field — description included — and returns "the book
about X" ahead of X. The author is added as a qualifier when known; it's what
separates homonyms, the problem that led to the guard in #108.

It doesn't decide identity: what validates if the volume is REALLY the requested item is enrich
(titleMatches/authorMatches). Here we only sort and translate the format.
*/
func (c *Client) Search(ctx context.Context, title, author string, year int) (Book, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Book{}, ErrNotFound
	}
	terms := []string{`intitle:"` + sanitize(title) + `"`}
	if a := sanitize(strings.TrimSpace(author)); a != "" {
		terms = append(terms, `inauthor:"`+a+`"`)
	}
	q := url.Values{}
	q.Set("q", strings.Join(terms, "+"))
	q.Set("maxResults", "10")
	q.Set("printType", "books")
	// projection=lite reduces the response size; imageLinks comes in it.
	q.Set("projection", "lite")
	if k := strings.TrimSpace(c.keyFn()); k != "" {
		q.Set("key", k)
	}

	// The `+` between qualifiers must survive encoding (Encode() would turn it
	// into %2B, which the API treats as a literal instead of a term separator).
	endpoint := c.base + "/books/v1/volumes?" + strings.ReplaceAll(q.Encode(), "%2B", "+")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Book{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.doWithRetry(ctx, req)
	if err != nil {
		return Book{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var raw struct {
		Items []struct {
			VolumeInfo struct {
				Title         string   `json:"title"`
				Subtitle      string   `json:"subtitle"`
				Authors       []string `json:"authors"`
				PublishedDate string   `json:"publishedDate"` // "2026", "2026-03", "2026-03-14"
				Categories    []string `json:"categories"`
				Language      string   `json:"language"`
				ImageLinks    struct {
					Thumbnail      string `json:"thumbnail"`
					SmallThumbnail string `json:"smallThumbnail"`
				} `json:"imageLinks"`
			} `json:"volumeInfo"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Book{}, fmt.Errorf("invalid Google Books response: %w", err)
	}
	if len(raw.Items) == 0 {
		return Book{}, ErrNotFound
	}

	// Prefers a volume WITH a cover (no cover is useless for the card) and, among those with covers, the one
	// from the requested year when provided — a useful tiebreaker for reissues.
	best := -1
	bestScore := -1
	for i := range raw.Items {
		v := raw.Items[i].VolumeInfo
		cover := v.ImageLinks.Thumbnail
		if cover == "" {
			cover = v.ImageLinks.SmallThumbnail
		}
		if cover == "" {
			continue
		}
		score := 1
		if year > 0 && yearOf(v.PublishedDate) == year {
			score += 2
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return Book{}, ErrNotFound
	}

	v := raw.Items[best].VolumeInfo
	cover := v.ImageLinks.Thumbnail
	if cover == "" {
		cover = v.ImageLinks.SmallThumbnail
	}
	out := Book{
		Title:    v.Title,
		Subtitle: v.Subtitle,
		Year:     yearOf(v.PublishedDate),
		// The API returns http:// in imageLinks; the Portal is served over https and the browser
		// would block it as mixed content — hence the scheme swap.
		CoverURL: strings.Replace(cover, "http://", "https://", 1),
		Language: v.Language,
	}
	if len(v.Authors) > 0 {
		out.Author = v.Authors[0]
		if len(v.Authors) > 1 {
			out.Author += " et al."
		}
	}
	if len(v.Categories) > 4 {
		out.Subjects = v.Categories[:4] // Google's category is noisy, just like OL's subject
	} else {
		out.Subjects = v.Categories
	}
	return out, nil
}

// Wait between doWithRetry attempts. Intentionally short: the enrichment runs
// inside the browse request, with a per-call timeout in enrich — sleeping too much here
// delivers the page without a poster anyway, just slower.
var retryWaits = []time.Duration{400 * time.Millisecond, 1200 * time.Millisecond}

/*
doWithRetry retries the search on TRANSIENT failures (429 and 5xx).

It exists because enrich enriches the catalog page in parallel (8 goroutines): a
page with dozens of books turns into a burst of searches, and Google Books responds with 429 or
503 for some of them. Without retry, this 503 silently became "no cover" — which is what
happened with "O Alfaiate Valentão" and "Night Songs", which the API DOES have with covers (#110).

4xx other than 429 is not retried: it's a malformed request or invalid key, and insisting only
wastes quota.
*/
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var last error
	for attempt := 0; ; attempt++ {
		resp, err := c.http.Do(req.Clone(ctx))
		switch {
		case err != nil:
			last = fmt.Errorf("search on Google Books: %w", err)
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return resp, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			_ = resp.Body.Close()
			last = fmt.Errorf("search on Google Books: status %d (transient)", resp.StatusCode)
		default:
			_ = resp.Body.Close()
			return nil, fmt.Errorf("search on Google Books: status %d", resp.StatusCode)
		}
		if attempt >= len(retryWaits) {
			return nil, last
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryWaits[attempt]):
		}
	}
}

// sanitize strips what would break the quoted qualifier in the query.
func sanitize(s string) string {
	return strings.NewReplacer(`"`, "", `\`, "", "+", " ").Replace(s)
}

// yearOf extracts the year from a publishedDate ("2026", "2026-03", "2026-03-14").
func yearOf(s string) int {
	if len(s) < 4 {
		return 0
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil || y < 1000 || y > 2200 {
		return 0
	}
	return y
}
