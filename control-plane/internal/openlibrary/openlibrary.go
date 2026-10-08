// Package openlibrary is a minimal client for the Open Library public API
// (openlibrary.org), used to enrich the Explore screen catalog (#12) with
// BOOKS and AUDIOBOOKS metadata — the equivalent of tmdb (movies/series) and igdb
// (games). Public API with no authentication: no key to configure.
//
// Since UNIT3D doesn't return a book id on browse, the search is heuristic by
// title+author+year (the operator uses the text already in the release name). The
// ranking prefers docs with a cover AND matching author+year, avoiding false positives.
package openlibrary

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

// ErrNotFound: no sufficiently reliable result.
var ErrNotFound = errors.New("no results on Open Library")

const (
	defaultBase = "https://openlibrary.org"
	// Cover ID → cover URL (size M ≈ 180×270, fits on the Explore screen card).
	coverBase = "https://covers.openlibrary.org/b/id/"
	coverSize = "-M.jpg"
)

// Book is a subset of the Open Library doc with only what the UI uses.
type Book struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Author   string   `json:"author"` // first author (the rest becomes "et al." if relevant)
	Year     int      `json:"year"`   // first_publish_year
	CoverURL string   `json:"cover_url"`
	Subjects []string `json:"subjects"` // used as "genres" — Open Library subjects are noisy, we cut it to the first 4
	Language string   `json:"language"` // ISO 639-1 if available (e.g., "en", "pt")
}

// Client talks to Open Library. No credentials — no state or configuration.
type Client struct {
	base string
	http *http.Client
}

// NewClient creates a client. The default Base URL is production.
func NewClient() *Client {
	return &Client{
		base: defaultBase,
		http: &http.Client{Timeout: 8 * time.Second},
	}
}

// WithBase swaps the base URL (used in tests).
func (c *Client) WithBase(base string) *Client {
	if base != "" {
		c.base = strings.TrimRight(base, "/")
	}
	return c
}

// Enabled is always true — Open Library doesn't need a key. It exists to keep the
// same "look" as the tmdb/igdb clients (the handler calls Enabled before triggering).
func (c *Client) Enabled() bool { return true }

// Search finds the most likely book for the title+author+year trio. Author and year are
// optional but GREATLY improve the match (Open Library indexes a lot of junk and returns
// the "book about the book on X" ahead of X). No reliable match → ErrNotFound.
func (c *Client) Search(ctx context.Context, title, author string, year int) (Book, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Book{}, ErrNotFound
	}
	q := url.Values{}
	q.Set("title", title)
	if a := strings.TrimSpace(author); a != "" {
		q.Set("author", a)
	}
	q.Set("limit", "10")
	// Restricts the fields to make the response smaller.
	q.Set("fields", "title,subtitle,author_name,first_publish_year,cover_i,subject,language")

	endpoint := c.base + "/search.json?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Book{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Book{}, fmt.Errorf("openlibrary: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Book{}, fmt.Errorf("openlibrary: status %d", resp.StatusCode)
	}

	var parsed struct {
		Docs []struct {
			Title            string   `json:"title"`
			Subtitle         string   `json:"subtitle"`
			AuthorName       []string `json:"author_name"`
			FirstPublishYear int      `json:"first_publish_year"`
			CoverI           int      `json:"cover_i"`
			Subject          []string `json:"subject"`
			Language         []string `json:"language"` // MARC (e.g.: "eng","por","jpn")
		} `json:"docs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Book{}, fmt.Errorf("openlibrary: invalid response: %w", err)
	}
	if len(parsed.Docs) == 0 {
		return Book{}, ErrNotFound
	}

	// Ranking: cover + matching year + matching author + matching title. The author's
	// last name alone is already a strong anchor (releases often write "Brothers
	// Grimm", but Open Library indexes "Jacob Grimm" or "Brothers Grimm").
	bestIdx, bestScore := -1, -1
	bestAnchored := false
	titleLower := strings.ToLower(title)
	authorLower := strings.ToLower(strings.TrimSpace(author))
	// Last word of the author = last name, stable token (Rowling, Grimm, Tolkien).
	// "J.K. Rowling" → "rowling"; "Brothers Grimm" → "grimm".
	var authorSurname string
	if authorLower != "" {
		toks := strings.Fields(authorLower)
		if len(toks) > 0 {
			authorSurname = toks[len(toks)-1]
		}
	}
	for i, d := range parsed.Docs {
		s := 0
		// anchored: match has at least ONE anchor that is not a title substring.
		// Without this, we'd accept "books about X" when asking for "X" — a common word
		// in a title matches everything (typical Open Library false positive).
		anchored := false
		if d.CoverI > 0 {
			s += 2 // cover is the main thing — without it the card looks ugly
			anchored = true
		}
		if year > 0 && d.FirstPublishYear == year {
			s += 3
			anchored = true
		} else if year > 0 && absInt(d.FirstPublishYear-year) <= 2 {
			s++ // close year (reissue/translation) — weak anchor, not anchored
		}
		if authorLower != "" {
			for _, a := range d.AuthorName {
				al := strings.ToLower(a)
				if strings.Contains(al, authorLower) {
					s += 3
					anchored = true
					break
				}
				if authorSurname != "" && strings.Contains(al, authorSurname) {
					s += 2 // last name matches — strong anchor even with a different first name
					anchored = true
					break
				}
			}
		}
		if strings.EqualFold(strings.TrimSpace(d.Title), title) {
			s += 3
		} else if strings.Contains(strings.ToLower(d.Title), titleLower) {
			s += 2
		} else if titleLower != "" && strings.Contains(titleLower, strings.ToLower(d.Title)) {
			s++ // doc title is contained in what we asked for (subtitle, anthology)
		}
		if s > bestScore {
			bestScore = s
			bestIdx = i
			bestAnchored = anchored
		}
	}
	// Threshold: minimum score 2 AND needs an anchor (cover, exact year, or author).
	// Without an anchor, we'd accept any doc whose title contains a word from
	// what we asked for — typical false positive. With an anchor + score 2 (e.g., just
	// cover + partial title), the UI already shows something minimally related.
	if bestIdx < 0 || bestScore < 2 || !bestAnchored {
		return Book{}, ErrNotFound
	}
	d := parsed.Docs[bestIdx]

	out := Book{Title: d.Title, Subtitle: d.Subtitle, Year: d.FirstPublishYear}
	if len(d.AuthorName) > 0 {
		out.Author = d.AuthorName[0]
		if len(d.AuthorName) > 1 {
			out.Author += " et al."
		}
	}
	if d.CoverI > 0 {
		out.CoverURL = coverBase + strconv.Itoa(d.CoverI) + coverSize
	}
	if len(d.Subject) > 0 {
		// Open Library Subjects are noisy ("fiction, general", "juvenile", …). Cuts
		// to the first 4 — fits in the UI and avoids chip-spam.
		n := len(d.Subject)
		if n > 4 {
			n = 4
		}
		out.Subjects = append(out.Subjects, d.Subject[:n]...)
	}
	if len(d.Language) > 0 {
		out.Language = marcToISO(d.Language[0])
	}
	return out, nil
}

// marcToISO translates the MARC code (3 letters) used by Open Library to the ISO 639-1
// (2 letters) that the UI shows. We only cover common languages; unknown comes back as
// it came so the operator can at least see the label.
func marcToISO(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "eng":
		return "en"
	case "por":
		return "pt"
	case "spa":
		return "es"
	case "fre", "fra":
		return "fr"
	case "ger", "deu":
		return "de"
	case "ita":
		return "it"
	case "jpn":
		return "ja"
	case "rus":
		return "ru"
	case "chi", "zho":
		return "zh"
	case "kor":
		return "ko"
	default:
		return strings.ToLower(strings.TrimSpace(code))
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
