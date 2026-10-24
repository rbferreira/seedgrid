// Package torrentfile creates .torrent files (BitTorrent v1) from data on disk, for an Engine that
// cannot (ADR-045 §5): the Agent hashes the data and the Engine loads the result pointing at it,
// which is what publishing (ADR-019) gets from qBittorrent's creator.
package torrentfile

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // G505: BitTorrent v1 pieces are SHA-1, by the protocol
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Options are what the .torrent carries besides the data.
type Options struct {
	Trackers []string // one tier each, in order; the first is also `announce`
	Private  bool
	Source   string // the tracker's tag (ADR-020); empty = none
}

// Piece size limits. The size is the smallest power of two that keeps the torrent near
// targetPieces pieces, as libtorrent's automatic choice (qBittorrent's default) does.
const (
	minPiece     = 16 << 10
	maxPiece     = 16 << 20
	targetPieces = 2048
)

// file is one file of the torrent: where it is on disk and its path inside the torrent.
type file struct {
	disk   string
	path   []string
	length int64
}

// Create builds the .torrent of path, a file or a folder.
func Create(ctx context.Context, path string, o Options) ([]byte, error) {
	root := filepath.Clean(path)
	st, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	var files []file
	if st.Mode().IsRegular() {
		files = []file{{disk: root, length: st.Size()}}
	} else if st.IsDir() {
		if files, err = walk(root); err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("torrentfile: %s has no files", root)
		}
	} else {
		return nil, fmt.Errorf("torrentfile: %s is neither a file nor a folder", root)
	}
	var total int64
	for _, f := range files {
		total += f.length
	}
	if total == 0 {
		return nil, fmt.Errorf("torrentfile: %s is empty", root)
	}
	piece := PieceSize(total)
	pieces, err := hashPieces(ctx, files, piece)
	if err != nil {
		return nil, err
	}

	var info bytes.Buffer
	info.WriteByte('d')
	if st.IsDir() {
		str(&info, "files")
		info.WriteByte('l')
		for _, f := range files {
			info.WriteByte('d')
			str(&info, "length")
			num(&info, f.length)
			str(&info, "path")
			info.WriteByte('l')
			for _, p := range f.path {
				str(&info, p)
			}
			info.WriteString("ee")
		}
		info.WriteByte('e')
	} else {
		str(&info, "length")
		num(&info, total)
	}
	str(&info, "name")
	str(&info, filepath.Base(root))
	str(&info, "piece length")
	num(&info, piece)
	str(&info, "pieces")
	str(&info, string(pieces))
	if o.Private {
		str(&info, "private")
		num(&info, 1)
	}
	if o.Source != "" {
		str(&info, "source")
		str(&info, o.Source)
	}
	info.WriteByte('e')

	var t bytes.Buffer
	t.WriteByte('d')
	if len(o.Trackers) > 0 {
		str(&t, "announce")
		str(&t, o.Trackers[0])
	}
	if len(o.Trackers) > 1 {
		str(&t, "announce-list")
		t.WriteByte('l')
		for _, tr := range o.Trackers {
			t.WriteByte('l')
			str(&t, tr)
			t.WriteByte('e')
		}
		t.WriteByte('e')
	}
	str(&t, "created by")
	str(&t, "SeedGrid")
	str(&t, "creation date")
	num(&t, time.Now().Unix())
	str(&t, "info")
	t.Write(info.Bytes())
	t.WriteByte('e')
	return t.Bytes(), nil
}

// PieceSize is the piece length for total bytes of data.
func PieceSize(total int64) int64 {
	p := int64(minPiece)
	for p < maxPiece && total/p > targetPieces {
		p *= 2
	}
	return p
}

// walk lists the regular files under root, in the order the torrent lists them: by path, as
// bytes. Links and other special files are left out.
func walk(root string) ([]file, error) {
	var files []file
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, file{disk: p, path: strings.Split(filepath.ToSlash(rel), "/"), length: info.Size()})
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.Join(files[i].path, "/") < strings.Join(files[j].path, "/")
	})
	return files, err
}

// hashPieces reads the files end to end, as one stream, and returns the SHA-1 of each piece.
func hashPieces(ctx context.Context, files []file, piece int64) ([]byte, error) {
	buf := make([]byte, piece)
	var out []byte
	fill := 0
	for _, f := range files {
		r, err := os.Open(f.disk)
		if err != nil {
			return nil, err
		}
		for {
			if err := ctx.Err(); err != nil {
				_ = r.Close()
				return nil, err
			}
			n, err := io.ReadFull(r, buf[fill:])
			fill += n
			if fill == len(buf) {
				sum := sha1.Sum(buf) //nolint:gosec // G401: BitTorrent v1 pieces are SHA-1
				out = append(out, sum[:]...)
				fill = 0
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			if err != nil {
				_ = r.Close()
				return nil, err
			}
		}
		if err := r.Close(); err != nil {
			return nil, err
		}
	}
	if fill > 0 {
		sum := sha1.Sum(buf[:fill]) //nolint:gosec // G401: BitTorrent v1 pieces are SHA-1
		out = append(out, sum[:]...)
	}
	return out, nil
}

func str(b *bytes.Buffer, s string) {
	b.WriteString(strconv.Itoa(len(s)))
	b.WriteByte(':')
	b.WriteString(s)
}

func num(b *bytes.Buffer, n int64) {
	b.WriteByte('i')
	b.WriteString(strconv.FormatInt(n, 10))
	b.WriteByte('e')
}
