// Package mediafile locates the main media file of an Engine's ContentPath
// (which can be a directory with multiple files or a single file) —
// heuristic shared by mediainfo (ADR-020) and screenshots
// (ADR-026).
package mediafile

import (
	"os"
	"path/filepath"
)

// LargestFile returns the path to the largest regular file under root (recursively),
// or "" if there is none. It works for both a directory and a single
// file, since filepath.Walk calls walkFn only once when root is already a
// file.
func LargestFile(root string) string {
	var best string
	var bestSize int64 = -1
	_ = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !fi.Mode().IsRegular() {
			return nil
		}
		if fi.Size() > bestSize {
			bestSize, best = fi.Size(), path
		}
		return nil
	})
	return best
}
