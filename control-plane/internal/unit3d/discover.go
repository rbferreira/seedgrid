package unit3d

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// Taxonomy discovery (#196, ADR-037): the UNIT3D TorrentResource returns, in the same
// catalog item, the NAME and ID of each axis (category/category_id, type/type_id,
// resolution/resolution_id). Crawling a few pages of the tracker's own catalog, with
// the operator's own token, builds the ID table without third-party login and without copying
// anyone else's table — IDs vary by site (Encode is 3 on one site and 12 on another),
// and that's why they can't be hardcoded in the product.
//
// The LIMIT the caller has to communicate: discovery only sees what is in the
// catalog. Categories that no one has ever posted don't appear, and the catalog comes from the most
// recent to the oldest — so the result is always an INCOMPLETE profile by
// definition, never "the site's profile".

// Taxonomy axes that the UNIT3D catalog exposes per item.
const (
	AxisCategory   = "category"
	AxisType       = "type"
	AxisResolution = "resolution"
)

// TaxonomyAxes lists the axes that Discover harvests, in stable order.
func TaxonomyAxes() []string { return []string{AxisCategory, AxisType, AxisResolution} }

const (
	// DefaultDiscoverPages is how many pages (of 50 items) discovery crawls by default.
	// Measured on a UNIT3D site on 03/10/2026: ~1.3 s per page, and 3 pages (150 items) already
	// fetched 5 categories, 5 types and 5 resolutions — 8 pages cover what is in use
	// with margin without exceeding ~11 s.
	DefaultDiscoverPages = 8
	// MaxDiscoverPages is the ceiling. What actually limits it is the time budget
	// (DiscoverBudget): the Control Plane HTTP server closes the response in 30 s, and the
	// response must fit within that so the operator receives the proposal instead of a cutoff.
	MaxDiscoverPages = 20
	// DiscoverBudget is the maximum time for the crawl. If it exceeds it, it returns what it harvested.
	DiscoverBudget = 20 * time.Second
)

// StopReason says why the crawl stopped. Only "end_of_catalog" means the tracker
// has no more pages; in all others, there is catalog that was not seen.
type StopReason string

const (
	StopMaxPages     StopReason = "max_pages"
	StopEndOfCatalog StopReason = "end_of_catalog"
	StopRateLimited  StopReason = "rate_limited"
	StopTimeBudget   StopReason = "time_budget"
	StopError        StopReason = "error"
)

// ObservedTerm is an (id, name) pair seen in the catalog, with how many times it appeared. Seen
// helps the operator judge: a name seen 1 time in 400 items deserves more skepticism
// than one seen 200 times.
type ObservedTerm struct {
	ID   int
	Name string
	Seen int
}

// Discovery is the result of a crawl. Partial is a valid result: if page 5
// failed, pages 1 to 4 still count — only Stopped/Err say it stopped early.
type Discovery struct {
	// Axes: axis → distinct (id, name) pairs, sorted by id and then by name. The same
	// id can appear with two names if the admin renamed it in the middle of the crawl; both
	// pairs come out and the consumer decides.
	Axes         map[string][]ObservedTerm
	PagesScanned int
	ItemsScanned int
	Stopped      StopReason
	// Err is the error that interrupted the crawl (nil when it stopped due to completeness/ceiling/deadline).
	Err error
}

// DiscoverOptions adjusts the scan. Zero = the default.
type DiscoverOptions struct {
	MaxPages int
	Budget   time.Duration
}

// PageFetcher fetches a catalog page by cursor (the page number, "" = 1st).
// It is the test seam: in production it is Browse.
type PageFetcher func(ctx context.Context, cursor string) (BrowseResult, error)

// DiscoverTaxonomy crawls the tracker catalog via GET /api/torrents/filter (the same
// endpoint as Browse) and aggregates the observed taxonomy.
func DiscoverTaxonomy(ctx context.Context, apiURL, apiToken string, opts DiscoverOptions) Discovery {
	return Discover(ctx, func(ctx context.Context, cursor string) (BrowseResult, error) {
		return Browse(ctx, apiURL, apiToken, cursor, "")
	}, opts)
}

// Discover crawls up to opts.MaxPages pages, sequentially (one page at a time: UNIT3D
// rate limits per token, and discovery shares the quota with the Portal), and aggregates
// the (id, name) pairs per axis. It never writes anything: it returns what it saw.
//
// Items with id <= 0 or empty name on an axis are left out of it — UNIT3D sends null for
// what is optional (resolution, for example), and null is not a term.
func Discover(ctx context.Context, fetch PageFetcher, opts DiscoverOptions) Discovery {
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultDiscoverPages
	}
	if maxPages > MaxDiscoverPages {
		maxPages = MaxDiscoverPages
	}
	budget := opts.Budget
	if budget <= 0 {
		budget = DiscoverBudget
	}
	bctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	type key struct {
		axis string
		id   int
		name string
	}
	seen := map[key]int{}
	count := func(axis string, id int, name string) {
		name = strings.TrimSpace(name)
		if id <= 0 || name == "" {
			return
		}
		seen[key{axis, id, name}]++
	}
	d := Discovery{Axes: map[string][]ObservedTerm{}}
	for _, a := range TaxonomyAxes() {
		d.Axes[a] = []ObservedTerm{}
	}

	cursor := ""
	for {
		if d.PagesScanned >= maxPages {
			d.Stopped = StopMaxPages
			break
		}
		res, err := fetch(bctx, cursor)
		if err != nil {
			switch {
			case errors.Is(err, ErrRateLimited):
				d.Stopped = StopRateLimited
			case ctx.Err() == nil && bctx.Err() != nil:
				// The deadline is for the crawl, not the caller: whatever came up to here counts.
				d.Stopped = StopTimeBudget
			default:
				d.Stopped = StopError
			}
			d.Err = err
			break
		}
		d.PagesScanned++
		for _, it := range res.Items {
			d.ItemsScanned++
			count(AxisCategory, it.CategoryID, it.Category)
			count(AxisType, it.TypeID, it.Type)
			count(AxisResolution, it.ResolutionID, it.Resolution)
		}
		if res.NextCursor == "" {
			d.Stopped = StopEndOfCatalog
			break
		}
		cursor = res.NextCursor
	}

	for k, n := range seen {
		d.Axes[k.axis] = append(d.Axes[k.axis], ObservedTerm{ID: k.id, Name: k.name, Seen: n})
	}
	for _, terms := range d.Axes {
		sort.Slice(terms, func(i, j int) bool {
			if terms[i].ID != terms[j].ID {
				return terms[i].ID < terms[j].ID
			}
			return terms[i].Name < terms[j].Name
		})
	}
	return d
}
