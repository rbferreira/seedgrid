// Package screenshot extracts frames (JPEG) from a video file via ffmpeg/ffprobe,
// for publication on trackers that require screenshots in the upload description
// (ADR-026). Best-effort: an isolated failure at a timestamp does not abort the others.
package screenshot

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Extract returns up to `count` JPEG frames extracted from file, at timestamps
// evenly distributed between 10% and 90% of the video duration (avoids
// opening/logo and end credits). Error only if NO frame can be extracted
// (e.g.: missing ffmpeg/ffprobe, corrupted file).
func Extract(ctx context.Context, file string, count int) ([][]byte, error) {
	dur, err := probeDuration(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("get video duration: %w", err)
	}
	if dur <= 0 {
		return nil, fmt.Errorf("unknown video duration")
	}
	var frames [][]byte
	for _, ts := range timestamps(dur, count) {
		data, err := extractFrame(ctx, file, ts)
		if err != nil || len(data) == 0 {
			continue
		}
		frames = append(frames, data)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames extracted (ffmpeg unavailable or failed at all points)")
	}
	return frames, nil
}

// timestamps returns `count` points (seconds) evenly distributed between
// 10% and 90% of durationSec. count<=0 is treated as 1.
func timestamps(durationSec float64, count int) []float64 {
	if count <= 0 {
		count = 1
	}
	start, end := durationSec*0.1, durationSec*0.9
	if count == 1 {
		return []float64{(start + end) / 2}
	}
	step := (end - start) / float64(count-1)
	out := make([]float64, count)
	for i := range out {
		out[i] = start + step*float64(i)
	}
	return out
}

// probeDuration reads the total video duration (seconds) via ffprobe.
func probeDuration(ctx context.Context, file string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", //nolint:gosec // G204: file is the storage file already resolved internally, not direct external input
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		file,
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// extractFrame captures a JPEG frame from the video at timestamp atSeconds.
func extractFrame(ctx context.Context, file string, atSeconds float64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ffmpeg", //nolint:gosec // G204: same as probeDuration above
		"-ss", strconv.FormatFloat(atSeconds, 'f', 2, 64),
		"-i", file,
		"-frames:v", "1",
		"-q:v", "2",
		"-f", "image2",
		"-",
	).Output()
}
