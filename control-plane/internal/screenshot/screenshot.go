// Package screenshot orchestrates the extraction of frames by the Agent (command queue,
// ADR-014) and assembles the BBCode of the already hosted screenshots, for publishing to
// UNIT3D trackers (ADR-026).
package screenshot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/command"
)

// EnqueueExtract enqueues extract_screenshots on the torrent Node and returns the id of the
// command WITHOUT blocking (F1: the Portal tracks the "Extracting on Node" step via the
// command cycle, GET /commands/{id}). The rendering (imgbb + BBCode) happens later, when the command
// completes.
func EnqueueExtract(ctx context.Context, cmds *command.Service, nodeID, hash string, count int) (string, error) {
	payload, err := json.Marshal(struct {
		Hash            string `json:"hash"`
		ScreenshotCount int    `json:"screenshot_count"`
	}{Hash: hash, ScreenshotCount: count})
	if err != nil {
		return "", err
	}
	return cmds.Enqueue(ctx, nodeID, "extract_screenshots", payload)
}

// FramesFromCommand reads and decodes the frames from an ALREADY completed extract_screenshots
// command (the Portal only calls the rendering after seeing the command in DONE).
func FramesFromCommand(ctx context.Context, cmds *command.Service, cmdID string) ([][]byte, error) {
	res, err := cmds.ResultOf(ctx, cmdID)
	if err != nil {
		return nil, err
	}
	return decodeFrames(res)
}

// RequestFrames enqueues extract_screenshots on the torrent Node and blocks until the
// Agent completes (or fails/times out), returning the already decoded JPEG frames. Used
// by the one-step flow (button "📸 Generate screenshots"); the step bar form uses
// separate EnqueueExtract + FramesFromCommand.
func RequestFrames(ctx context.Context, cmds *command.Service, nodeID, hash string, count int) ([][]byte, error) {
	cmdID, err := EnqueueExtract(ctx, cmds, nodeID, hash, count)
	if err != nil {
		return nil, fmt.Errorf("queue screenshot extraction: %w", err)
	}
	st, err := cmds.WaitDone(ctx, cmdID, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	if st.Status == "FAILED" {
		if st.Error != nil {
			return nil, errors.New(*st.Error)
		}
		return nil, errors.New("screenshot extraction failed on Node")
	}
	return FramesFromCommand(ctx, cmds, cmdID)
}

// decodeFrames extracts and decodes the JPEG frames (base64) from the command result.
// Factored out from RequestFrames to be testable without needing a real
// command.Service (which depends on the database).
func decodeFrames(rawResult json.RawMessage) ([][]byte, error) {
	var out struct {
		Frames []string `json:"frames"`
	}
	if err := json.Unmarshal(rawResult, &out); err != nil {
		return nil, fmt.Errorf("invalid Node result: %w", err)
	}
	frames := make([][]byte, 0, len(out.Frames))
	for _, f := range out.Frames {
		data, err := base64.StdEncoding.DecodeString(f)
		if err != nil {
			continue
		}
		frames = append(frames, data)
	}
	if len(frames) == 0 {
		return nil, errors.New("no frame received from the Node")
	}
	return frames, nil
}

// Shot is an already hosted screenshot: the full image and the thumbnail (backlog F1). A type
// local to the package so it doesn't depend on `imgbb` — the caller converts it.
type Shot struct {
	Full  string
	Thumb string
}

// BuildBBCode assembles the screenshot block in BBCode in the conventional private tracker
// format (backlog F1): section header + CLICKABLE centered thumbnails
// (`[url=full][img]thumb[/img][/url]`, one per line). Consistent with the rest of the form
// (internal/description, which already uses `[center]`/`[b]section[/b]`) and
// avoids huge inline images — the common cause of "postponed" in moderation. Empty if
// there are no screenshots. When the thumbnail is missing, it uses the full image itself.
func BuildBBCode(shots []Shot) string {
	if len(shots) == 0 {
		return ""
	}
	lines := make([]string, len(shots))
	for i, s := range shots {
		thumb := s.Thumb
		if thumb == "" {
			thumb = s.Full
		}
		lines[i] = "[url=" + s.Full + "][img]" + thumb + "[/img][/url]"
	}
	return "[b]Screenshots[/b]\n[center]\n" + strings.Join(lines, "\n") + "\n[/center]"
}
