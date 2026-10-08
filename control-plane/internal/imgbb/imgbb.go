// Package imgbb is a client for the imgbb.com image host — used to host the
// screenshots that UNIT3D trackers require in the upload description (ADR-026/ADR-030).
//
// Unlike ptpimg (ADR-026, removed — went offline and never had an official API), imgbb
// has a public and documented v1 API: POST https://api.imgbb.com/1/upload?key=<API_KEY> with
// the `image` field in base64, JSON response `{data:{url:…}}`. One image per request.
package imgbb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// uploadURL is the upload endpoint. A variable (not const) so tests can point to an
// httptest.Server and restore it at the end (defer).
var uploadURL = "https://api.imgbb.com/1/upload"

// stripURL strips the URL from a *url.Error. The http.Client error carries the full URL, and
// here it carries the key in the query (`?key=<key>`). Without this trim, a network failure
// returns the operator's credential to the Portal and the log — the same leak that #220
// fixed in the UNIT3D client (ADR-040).
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// StatusError: imgbb responded with a non-2xx status. Typed so the host chain (ADR-040)
// can translate it to "HTTP <status>"; Message is the text from imgbb itself, when it sends one.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return "imgbb rejected upload: " + e.Message
	}
	return fmt.Sprintf("imgbb rejected upload (status %d)", e.Status)
}

// ErrNoURL: imgbb responded 2xx without the image URL.
var ErrNoURL = errors.New("imgbb refused upload: response without URL")

// Image is the result of an upload: the full image URL and the thumbnail URL (ADR-032
// backlog F1). The thumbnail allows the conventional clickable screenshot BBCode on a
// private tracker (`[url=full][img]thumb[/img][/url]`), avoiding huge inline images.
type Image struct {
	Full  string
	Thumb string
}

// uploadResponse is the imgbb envelope. `data.url` is the full image; `data.thumb.url`
// (or `data.medium.url`) is the thumbnail — imgbb does not always return both.
type uploadResponse struct {
	Data struct {
		URL   string `json:"url"`
		Thumb struct {
			URL string `json:"url"`
		} `json:"thumb"`
		Medium struct {
			URL string `json:"url"`
		} `json:"medium"`
	} `json:"data"`
	Success bool `json:"success"`
	Status  int  `json:"status"`
	Error   struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Upload sends each frame (JPEG) to imgbb and returns the images (full + thumbnail) in the
// SAME input order (the screenshots appear sequentially in the description). imgbb does
// one upload per request, so we iterate; a frame failure aborts with a clear error —
// better not to publish than to publish a half-finished description.
func Upload(ctx context.Context, apiKey string, frames [][]byte) ([]Image, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("imgbb api_key not configured")
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames to send")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	imgs := make([]Image, 0, len(frames))
	for i, f := range frames {
		img, err := uploadOne(ctx, client, apiKey, f)
		if err != nil {
			return nil, fmt.Errorf("frame %d: %w", i, err)
		}
		imgs = append(imgs, img)
	}
	return imgs, nil
}

func uploadOne(ctx context.Context, client *http.Client, apiKey string, frame []byte) (Image, error) {
	// `image` field = base64 of the JPEG (imgbb accepts binary, base64, or URL; base64 avoids
	// the percent-encoding overhead that a urlencoded POST would bring).
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("image", base64.StdEncoding.EncodeToString(frame)); err != nil {
		return Image{}, err
	}
	if err := mw.Close(); err != nil {
		return Image{}, err
	}

	// The key goes in the query (imgbb contract), not in the body.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL+"?key="+apiKey, &buf)
	if err != nil {
		return Image{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		// stripURL strips the full URL from *url.Error — it carries the key in the query
		// (ADR-040, same pattern as unit3d).
		return Image{}, fmt.Errorf("connect to imgbb: %w", stripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	var out uploadResponse
	if err := json.Unmarshal(body, &out); err != nil {
		// Non-JSON response (e.g., error HTML) or bad status: reports the status.
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return Image{}, &StatusError{Status: resp.StatusCode}
		}
		return Image{}, fmt.Errorf("invalid imgbb response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Image{}, &StatusError{Status: resp.StatusCode, Message: out.Error.Message}
	}
	if out.Data.URL == "" {
		return Image{}, ErrNoURL
	}
	// Thumbnail: prefer thumb, fall back to medium, and finally the full one (always exists).
	thumb := out.Data.Thumb.URL
	if thumb == "" {
		thumb = out.Data.Medium.URL
	}
	if thumb == "" {
		thumb = out.Data.URL
	}
	return Image{Full: out.Data.URL, Thumb: thumb}, nil
}
