// Package unit3d is a client for the upload endpoint of the UNIT3D v8 API (ADR-020),
// used to automatically register a release in a tracker's catalog (e.g.:
// tracker.example). It sends the .torrent already created by SeedGrid + the metadata reviewed
// by the operator. Auth = api_token in the query string.
package unit3d

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Params gathers the fields accepted by POST /api/torrents/upload. Zeroed fields are
// omitted when it makes sense (e.g.: season/episode only for series).
type Params struct {
	Name         string
	Description  string
	CategoryID   int
	TypeID       int
	ResolutionID int
	TMDB         int
	IMDB         string // "tt1375666" or number; the "tt" is removed on submission
	TVDB         int
	MAL          int
	IGDB         int // IGDB id (games, #17)
	Season       int
	Episode      int
	Keywords     string
	MediaInfo    string
	Anonymous    bool
}

// Result is the success response from UNIT3D.
type Result struct {
	DownloadURL string `json:"download_url"`
	Message     string `json:"message"`
}

// Fields builds the multipart text fields exactly as the POST sends them.
//
// It exists separately because UNIT3D **has no dry-run**: the only way to know what will
// be posted is by posting it. The preview (#188) reads from here, and Upload does too — if there were
// two builders, the preview could show one thing and the POST send another, which is
// precisely the failure the preview was supposed to eliminate.
//
// Zeroed fields that the tracker accepts as 0 go as is; optional ones (resolution,
// season, episode, keywords, mediainfo) are only included when they have a value.
func Fields(p Params) map[string]string {
	fields := map[string]string{
		"name":        p.Name,
		"description": p.Description,
		"category_id": itoa(p.CategoryID),
		"type_id":     itoa(p.TypeID),
		"tmdb":        itoa(p.TMDB),
		"imdb":        stripTT(p.IMDB),
		"tvdb":        itoa(p.TVDB),
		"mal":         itoa(p.MAL),
		"igdb":        itoa(p.IGDB),
		"anonymous":   boolToStr(p.Anonymous),
	}
	if p.ResolutionID > 0 {
		fields["resolution_id"] = itoa(p.ResolutionID)
	}
	if p.Season > 0 {
		fields["season_number"] = itoa(p.Season)
	}
	if p.Episode > 0 {
		fields["episode_number"] = itoa(p.Episode)
	}
	if strings.TrimSpace(p.Keywords) != "" {
		fields["keywords"] = p.Keywords
	}
	if strings.TrimSpace(p.MediaInfo) != "" {
		fields["mediainfo"] = p.MediaInfo
	}
	return fields
}

// Fingerprint identifies a publishing payload. It is what "freezes" the preview: the Portal
// receives this fingerprint along with the preview and returns it on submit; if the operator changed the
// form in the meantime, the fingerprint won't match and the submit is refused instead of
// posting something no one reviewed.
//
// It covers the text fields **and** the .torrent (name + size + content hash) — swapping
// the file while keeping the form identical would publish different content under the same review.
func Fingerprint(p Params, torrentName string, torrentBytes []byte) string {
	fields := Fields(p)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys) // map has no order; without this the fingerprint would change on every call
	h := sha256.New()
	// hash.Hash.Write never returns an error (stdlib contract) — the return exists only
	// to satisfy io.Writer, and that is why it is explicitly discarded.
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s\x00", k, fields[k])
	}
	_, _ = fmt.Fprintf(h, "torrent=%s\x00%d\x00", ensureTorrentExt(torrentName), len(torrentBytes))
	_, _ = h.Write(torrentBytes)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// Upload sends the .torrent + metadata to the UNIT3D tracker and returns the download URL
// (with passkey) that confirms the registration. Network/validation errors become an error.
func Upload(ctx context.Context, apiURL, apiToken string, p Params, torrentName string, torrentBytes []byte) (Result, error) {
	if strings.TrimSpace(apiURL) == "" || strings.TrimSpace(apiToken) == "" {
		return Result{}, fmt.Errorf("tracker without api_url/api_token configured")
	}
	if len(torrentBytes) == 0 {
		return Result{}, fmt.Errorf("no .torrent bytes to upload")
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// .torrent file.
	fw, err := mw.CreateFormFile("torrent", ensureTorrentExt(torrentName))
	if err != nil {
		return Result{}, err
	}
	if _, err := fw.Write(torrentBytes); err != nil {
		return Result{}, err
	}

	// Text fields — the SAME function the preview uses (#188). The preview is only worth
	// anything if there isn't a second path capable of diverging from this one.
	fields := Fields(p)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return Result{}, err
		}
	}
	if err := mw.Close(); err != nil {
		return Result{}, err
	}

	endpoint := strings.TrimRight(apiURL, "/") + "/api/torrents/upload?api_token=" + url.QueryEscape(apiToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return Result{}, stripURL(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("connect to tracker: %w", stripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	// Success response: {"data": <url>, "message": "..."}; error returns "message"/"errors".
	var parsed struct {
		Data    any             `json:"data"`
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := parsed.Message
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		if len(parsed.Errors) > 0 {
			msg = msg + " " + string(parsed.Errors)
		}
		return Result{}, fmt.Errorf("tracker rejected the upload (status %d): %s", resp.StatusCode, strings.TrimSpace(msg))
	}

	res := Result{Message: parsed.Message}
	if s, ok := parsed.Data.(string); ok {
		res.DownloadURL = s
	}
	return res, nil
}

// BrowseItem is a torrent from the tracker's catalog (a SAFE subset of the attributes —
// without `download_link`, which carries the passkey and stays only on the server). #12.
//
// The *_ids accompany the names in the same item (UNIT3D's TorrentResource) and are what
// allows DISCOVERING the site's taxonomy from the catalog (#196, ADR-037, discover.go).
type BrowseItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Type           string `json:"type"`
	Resolution     string `json:"resolution,omitempty"` // "1080p", "2160p", … when the tracker returns them
	CategoryID     int    `json:"category_id"`          // *_id: 0 when the tracker sends null (resolution is optional)
	TypeID         int    `json:"type_id"`
	ResolutionID   int    `json:"resolution_id"`
	Size           int64  `json:"size"`
	Seeders        int    `json:"seeders"`
	Leechers       int    `json:"leechers"`
	TimesCompleted int    `json:"times_completed"`
	TMDB           int    `json:"tmdb_id"`
	IMDB           string `json:"imdb_id"`
	IGDB           int    `json:"igdb_id"` // games (#17); 0 when the tracker sends null
	Year           int    `json:"release_year"`
	DetailsLink    string `json:"details_link"`
	// Files are the names of the item's files: together with the size, they are the evidence that
	// a catalog item is the SAME file the cluster already has, since the catalog doesn't expose the
	// info_hash and the item name isn't the torrent name (#37). Server-side only.
	Files []string `json:"-"`
}

// BrowseResult is a page from the catalog + the next page's cursor (cursor pagination).
// Links maps id→download_link (with passkey) — stays ONLY on the server (the CP caches it for the
// seed to use; it doesn't go to the Portal). #12.
type BrowseResult struct {
	Items      []BrowseItem      `json:"items"`
	NextCursor string            `json:"next_cursor"`
	Links      map[string]string `json:"-"`
}

// Browse lists the tracker's catalog (UNIT3D's JSON:API). UNIT3D DOES NOT expose info_hash
// here; the "already have" match is by exact size + name (torrent.Library.HasFile, #37) and the
// hard guarantee stays on add (duplicate guard, #11). #12.
//
// PAGE pagination via GET /api/torrents/filter (#13, bug #9): the `cursor`
// received/returned is the page NUMBER (string); the next page comes from
// meta.current_page + links.next. With `search`, it adds `name=…` (server-side filter).
// WITHOUT `search`, the filter without `name` returns the catalog (newest first),
// actually paginating — unlike GET /api/torrents, whose cursor in the Meilisearch
// driver of a real site was BROKEN (always returns the 1st page + the same
// next_cursor → "Load more" didn't advance, bug #9).
func Browse(ctx context.Context, apiURL, apiToken, cursor, search string) (BrowseResult, error) {
	if strings.TrimSpace(apiURL) == "" || strings.TrimSpace(apiToken) == "" {
		return BrowseResult{}, fmt.Errorf("tracker without api_url/api_token configured")
	}
	q := url.Values{}
	q.Set("api_token", apiToken)
	q.Set("perPage", "50")
	path := "/api/torrents/filter"
	if s := strings.TrimSpace(search); s != "" {
		q.Set("name", s) // server-side filter by name (#13)
	}
	page := strings.TrimSpace(cursor)
	if page == "" {
		page = "1"
	}
	q.Set("page", page)
	endpoint := strings.TrimRight(apiURL, "/") + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return BrowseResult{}, stripURL(err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return BrowseResult{}, fmt.Errorf("connect to tracker: %w", stripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		// Sentinel, so whoever sweeps multiple pages (taxonomy discovery, #196) stops
		// WITH what it has already collected instead of treating the rate limit as a total failure.
		return BrowseResult{}, fmt.Errorf("tracker rejected the listing: %w", ErrRateLimited)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BrowseResult{}, fmt.Errorf("tracker rejected the listing (status %d)", resp.StatusCode)
	}
	var parsed struct {
		Data []struct {
			ID         any `json:"id"`
			Attributes struct {
				// UNIT3D is loose with types: numeric fields can come as number OR string
				// (e.g.: release_year "2020"). We receive it as any and convert it.
				Name           string `json:"name"`
				Category       string `json:"category"`
				Type           string `json:"type"`
				Resolution     string `json:"resolution"` // optional; not every UNIT3D populates it
				CategoryID     any    `json:"category_id"`
				TypeID         any    `json:"type_id"`
				ResolutionID   any    `json:"resolution_id"`
				Size           any    `json:"size"`
				Seeders        any    `json:"seeders"`
				Leechers       any    `json:"leechers"`
				TimesCompleted any    `json:"times_completed"`
				TMDB           any    `json:"tmdb_id"`
				IMDB           any    `json:"imdb_id"`
				IGDB           any    `json:"igdb_id"` // games; sites almost always send null
				Year           any    `json:"release_year"`
				DetailsLink    string `json:"details_link"`
				DownloadLink   string `json:"download_link"`
				Files          []struct {
					Name string `json:"name"`
				} `json:"files"`
			} `json:"attributes"`
		} `json:"data"`
		Meta struct {
			CurrentPage any `json:"current_page"` // page pagination (/api/torrents/filter)
		} `json:"meta"`
		Links struct {
			Next any `json:"next"` // Next page URL (string) or null
		} `json:"links"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return BrowseResult{}, fmt.Errorf("invalid tracker response: %w", err)
	}
	out := BrowseResult{
		Items: make([]BrowseItem, 0, len(parsed.Data)),
		Links: make(map[string]string, len(parsed.Data)),
	}
	// Next page only when the tracker points to links.next AND items came (the UNIT3D
	// Meilisearch driver leaves meta.last_page/links.last null — we can't count).
	if hasValue(parsed.Links.Next) && len(parsed.Data) > 0 {
		page := anyToInt64(parsed.Meta.CurrentPage)
		if page < 1 {
			page = 1
		}
		out.NextCursor = strconv.FormatInt(page+1, 10)
	}
	for _, d := range parsed.Data {
		a := d.Attributes
		id := anyToStr(d.ID)
		files := make([]string, 0, len(a.Files))
		for _, f := range a.Files {
			files = append(files, f.Name)
		}
		out.Items = append(out.Items, BrowseItem{
			ID: id, Name: a.Name, Category: a.Category, Type: a.Type, Resolution: a.Resolution,
			CategoryID: anyToInt(a.CategoryID), TypeID: anyToInt(a.TypeID), ResolutionID: anyToInt(a.ResolutionID),
			Size:    anyToInt64(a.Size),
			Seeders: anyToInt(a.Seeders), Leechers: anyToInt(a.Leechers), TimesCompleted: anyToInt(a.TimesCompleted),
			TMDB: anyToInt(a.TMDB), IMDB: anyToStr(a.IMDB), IGDB: anyToInt(a.IGDB),
			Year: anyToInt(a.Year), DetailsLink: a.DetailsLink, Files: files,
		})
		if a.DownloadLink != "" {
			out.Links[id] = a.DownloadLink
		}
	}
	return out, nil
}

// FetchTorrent downloads the .torrent bytes of a catalog torrent (by id). It resolves the
// download_link (with passkey — server-side) via GET /api/torrents/{id} and downloads it; tries
// it as it came and, if it's not bencode, appends the api_token. #12.
func FetchTorrent(ctx context.Context, apiURL, apiToken, torrentID string) ([]byte, error) {
	if strings.TrimSpace(apiURL) == "" || strings.TrimSpace(apiToken) == "" {
		return nil, fmt.Errorf("tracker without api_url/api_token configured")
	}
	base := strings.TrimRight(apiURL, "/")
	cli := &http.Client{Timeout: 60 * time.Second}

	u := base + "/api/torrents/" + url.PathEscape(torrentID) + "?api_token=" + url.QueryEscape(apiToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, stripURL(err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return nil, networkFailure("query torrent on tracker", err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if err != nil {
		return nil, networkFailure("query torrent on tracker", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tracker rejected the torrent query (status %d)", resp.StatusCode)
	}
	var parsed struct {
		Data struct {
			Attributes struct {
				DownloadLink string `json:"download_link"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("invalid tracker response: %w", err)
	}
	link := parsed.Data.Attributes.DownloadLink
	if link == "" {
		return nil, fmt.Errorf("torrent without download_link on the tracker")
	}
	return DownloadFromLink(ctx, link, apiToken)
}

// DownloadFromLink downloads the .torrent bytes from a download_link (already with
// passkey) obtained from the catalog (#12): tries it as it came and, if it's not bencode, appends
// the api_token. It is the preferred path for the seed (the download_link comes in browse), avoiding
// the /api/torrents/{id} endpoint — which doesn't always bring the download_link.
func DownloadFromLink(ctx context.Context, link, apiToken string) ([]byte, error) {
	if strings.TrimSpace(link) == "" {
		return nil, fmt.Errorf("empty download_link")
	}
	cli := &http.Client{Timeout: 60 * time.Second}
	data, errDirect := fetchTorrentBytes(ctx, cli, link)
	if errDirect == nil && looksBencode(data) {
		return data, nil
	}
	// The tracker's final verdict DOES NOT become a second request: neither access refusal
	// nor rate limit change response for being repeated, and the second attempt
	// doubles the load against whoever is already denying — which is what produced the isolated 429 in the
	// production log on 10/09/2026. Repeating right after a 429 is the worst possible
	// move: it spends the quota you are waiting to refill (#207).
	var denied *DeniedError
	if errors.As(errDirect, &denied) || errors.Is(errDirect, ErrRateLimited) {
		return nil, errDirect
	}
	// A timeout ends it here too. The token doesn't speed up a tracker that isn't answering, and
	// the caller's deadline is already gone: the second attempt would start cancelled and replace
	// the real reason with "context canceled" in the report (#35).
	if errors.Is(errDirect, ErrTimeout) {
		return nil, errDirect
	}
	sep := "?"
	if strings.Contains(link, "?") {
		sep = "&"
	}
	data, err := fetchTorrentBytes(ctx, cli, link+sep+"api_token="+url.QueryEscape(apiToken))
	if err != nil {
		// The error from the first attempt is no longer swallowed: without it, a network failure
		// on the path without token disappears from the report and the operator only sees the symptom of the second.
		if errDirect != nil {
			return nil, fmt.Errorf("%w (without api_token: %v)", err, errDirect)
		}
		return nil, err
	}
	if !looksBencode(data) {
		return nil, fmt.Errorf("downloaded content is not a valid .torrent (%s)", describesBody(data))
	}
	return data, nil
}

// ErrRateLimited is the tracker's 429, a sentinel so the upper layer can decide
// to wait instead of insisting. Separated from DeniedError because the difference is the remedy:
// refusal requires intervention on the account, rate limit requires patience.
var ErrRateLimited = errors.New("the tracker rate-limited the requests (429)")

// ErrTimeout says the tracker didn't answer within the deadline — the http.Client's or the
// caller's context. A sentinel so the route answers 504 with the reason instead of a generic
// 502: a slow tracker calls for waiting and trying again, not for checking the account (#35).
var ErrTimeout = errors.New("the tracker did not answer in time")

// networkFailure translates the error of a request to the tracker: a timeout becomes ErrTimeout
// (the "awaiting headers" detail doesn't help the operator); anything else gets the operation
// prefix, without the URL, which carries a credential (stripURL).
func networkFailure(op string, err error) error {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return ErrTimeout
	}
	return fmt.Errorf("%s: %w", op, stripURL(err))
}

// DeniedError says the tracker REFUSED the download instead of delivering the file: the
// response redirected to another page (typically /login or the torrent details page).
//
// It is its own type because the distinction changes where the operator looks. On 10/09/2026 a
// site started refusing specific torrents like this, and since Go's http.Client
// follows redirects by default, what arrived here was the login page HTML with
// status 200 — rejected by looksBencode and reported as "invalid .torrent". The
// message told us to investigate the file when there was never a file, and it cost hours of
// investigation in the wrong direction (bencode, endpoint, rate limit, freeleech).
type DeniedError struct {
	// FinalPath is just the path of the final URL, never the entire URL: the
	// download_link query carries the user's passkey and must not end up in logs.
	FinalPath string
	Status    int
}

func (e *DeniedError) Error() string {
	return fmt.Sprintf("the tracker denied the download and redirected to %s (status %d) — "+
		"not a file issue: check permission, download quota or moderation "+
		"of the tracker account", e.FinalPath, e.Status)
}

func fetchTorrentBytes(ctx context.Context, cli *http.Client, link string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, stripURL(err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, networkFailure("download .torrent", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
			return nil, fmt.Errorf("%w; Retry-After: %s", ErrRateLimited, ra)
		}
		return nil, ErrRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(".torrent download failed (status %d)", resp.StatusCode)
	}
	// The client follows redirects; ending up on ANOTHER path means the tracker didn't
	// deliver the file, it delivered a page. Comparing the final path with the requested one is
	// what separates "access denied" from "corrupted file".
	if p, ok := pathOf(link); ok && resp.Request != nil && resp.Request.URL != nil &&
		resp.Request.URL.Path != p {
		return nil, &DeniedError{FinalPath: resp.Request.URL.Path, Status: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, networkFailure("download .torrent", err)
	}
	return data, nil
}

// pathOf extracts the Path from a URL. ok=false when it cannot be parsed — in this case
// the redirect check is skipped, instead of falsely reporting a refusal.
func pathOf(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	return u.Path, true
}

// describesBody summarizes a body that is not bencode, so the error says what arrived instead
// of just saying what it isn't. It truncates and removes breaks to fit in a single log line.
func describesBody(b []byte) string {
	if len(b) == 0 {
		return "empty response"
	}
	start := b
	if len(start) > 40 {
		start = start[:40]
	}
	clean := strings.Join(strings.Fields(string(start)), " ")
	return fmt.Sprintf("%d bytes, starts with %q", len(b), clean)
}

// stripURL strips the URL from a *url.Error. The http.Client error (and url.Parse's, when
// building the request) carries the ENTIRE URL, and here it takes the api_token in the query — and, in
// download_link, the user's passkey. Without this cut, "network failure" becomes the operator's
// credential in the message returned to the Portal and written to the log (#220). Any error from
// a tracker request goes through here: Upload, Browse, FetchTorrent and fetchTorrentBytes.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// looksBencode: a .torrent starts with 'd' (bencode dictionary).
func looksBencode(b []byte) bool { return len(b) > 0 && b[0] == 'd' }

// anyToInt/anyToInt64 convert a JSON field that can come as number OR string OR null
// (UNIT3D is inconsistent — e.g.: release_year "2020"). Invalid/missing → 0.
func anyToInt(v any) int { return int(anyToInt64(v)) }

func anyToInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case json.Number:
		i, _ := x.Int64()
		return i
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}

// hasValue tells if an any JSON field has non-empty content (used for links.next,
// which comes as a URL string when there is a next page or null when it's the last).
func hasValue(v any) bool { return strings.TrimSpace(anyToStr(v)) != "" }

// anyToStr converts a JSON field that can come as number OR string (e.g.: id, imdb_id).
func anyToStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case json.Number:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func boolToStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// stripTT removes the "tt" prefix from the IMDB id and returns only the number (UNIT3D stores
// imdb as integer). "" or non-numeric → "0".
func stripTT(imdb string) string {
	imdb = strings.TrimSpace(imdb)
	imdb = strings.TrimPrefix(strings.ToLower(imdb), "tt")
	imdb = strings.TrimLeft(imdb, "0")
	if imdb == "" {
		return "0"
	}
	if _, err := strconv.Atoi(imdb); err != nil {
		return "0"
	}
	return imdb
}

// ensureTorrentExt ensures a filename with a .torrent extension.
func ensureTorrentExt(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "upload"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
		name += ".torrent"
	}
	return name
}
