// Package torrentfile does a minimal parse of a .torrent file (bencode) to
// extract the infohash (v1), name, and trackers — enough for the Control
// Plane to register the torrent and pass the bytes to the Agent/Engine.
package torrentfile

import (
	"crypto/sha1" //nolint:gosec // G505: SHA1 is the infohash hash required by the BitTorrent v1 protocol, not for cryptographic use
	"encoding/hex"
	"errors"
	"fmt"
)

// Meta is the metadata extracted from a .torrent.
type Meta struct {
	InfoHash string   // sha1 of the info dictionary (hex, 40 chars)
	Name     string   // info.name
	Trackers []string // announce + announce-list
	Size     int64    // sum of content bytes (single: info.length; multi: Σ files[].length). 0 if absent. Basis of the disk check (ADR-032).
}

var errBencode = errors.New("invalid bencode")

// Parse extracts the metadata from a .torrent. It does not validate the content beyond
// what is necessary for the infohash/name/trackers.
func Parse(data []byte) (Meta, error) {
	if len(data) == 0 || data[0] != 'd' {
		return Meta{}, fmt.Errorf("%w: not a dictionary", errBencode)
	}
	top, _, err := decode(data, 0)
	if err != nil {
		return Meta{}, err
	}
	dict, ok := top.(map[string]any)
	if !ok {
		return Meta{}, fmt.Errorf("%w: top is not a dictionary", errBencode)
	}

	// InfoHash: sha1 of the raw span of the "info" value.
	start, end, err := infoSpan(data)
	if err != nil {
		return Meta{}, err
	}
	sum := sha1.Sum(data[start:end]) //nolint:gosec // G401: infohash of the BitTorrent protocol, not for cryptographic use
	m := Meta{InfoHash: hex.EncodeToString(sum[:])}

	if info, ok := dict["info"].(map[string]any); ok {
		if name, ok := info["name"].(string); ok {
			m.Name = name
		}
		// Total content size (ADR-032): single-file provides info.length; multi-file
		// sums files[].length. Absence (or unexpected type) leaves Size=0 → the disk
		// check skips instead of blocking (can't judge without the size).
		if n, ok := info["length"].(int); ok {
			m.Size = int64(n)
		} else if files, ok := info["files"].([]any); ok {
			for _, f := range files {
				if fm, ok := f.(map[string]any); ok {
					if l, ok := fm["length"].(int); ok && l > 0 {
						m.Size += int64(l)
					}
				}
			}
		}
	}
	if a, ok := dict["announce"].(string); ok && a != "" {
		m.Trackers = append(m.Trackers, a)
	}
	if list, ok := dict["announce-list"].([]any); ok {
		for _, tier := range list {
			if urls, ok := tier.([]any); ok {
				for _, u := range urls {
					if s, ok := u.(string); ok && s != "" {
						m.Trackers = append(m.Trackers, s)
					}
				}
			}
		}
	}
	m.Trackers = dedup(m.Trackers)
	return m, nil
}

// decode returns the bencode value at i and the next position.
func decode(data []byte, i int) (any, int, error) {
	if i >= len(data) {
		return nil, 0, errBencode
	}
	switch c := data[i]; {
	case c == 'i':
		j := indexByte(data, 'e', i+1)
		if j < 0 {
			return nil, 0, errBencode
		}
		var n int
		if _, err := fmt.Sscanf(string(data[i+1:j]), "%d", &n); err != nil {
			return nil, 0, errBencode
		}
		return n, j + 1, nil
	case c == 'l':
		i++
		var list []any
		for i < len(data) && data[i] != 'e' {
			v, ni, err := decode(data, i)
			if err != nil {
				return nil, 0, err
			}
			list = append(list, v)
			i = ni
		}
		if i >= len(data) {
			return nil, 0, errBencode
		}
		return list, i + 1, nil
	case c == 'd':
		i++
		d := map[string]any{}
		for i < len(data) && data[i] != 'e' {
			k, ni, err := decode(data, i)
			if err != nil {
				return nil, 0, err
			}
			ks, ok := k.(string)
			if !ok {
				return nil, 0, errBencode
			}
			v, ni2, err := decode(data, ni)
			if err != nil {
				return nil, 0, err
			}
			d[ks] = v
			i = ni2
		}
		if i >= len(data) {
			return nil, 0, errBencode
		}
		return d, i + 1, nil
	case c >= '0' && c <= '9':
		colon := indexByte(data, ':', i)
		if colon < 0 {
			return nil, 0, errBencode
		}
		var n int
		if _, err := fmt.Sscanf(string(data[i:colon]), "%d", &n); err != nil {
			return nil, 0, errBencode
		}
		start := colon + 1
		if start+n > len(data) {
			return nil, 0, errBencode
		}
		return string(data[start : start+n]), start + n, nil
	default:
		return nil, 0, errBencode
	}
}

// infoSpan locates the [start,end) span of the raw value of the "info" key in
// the top-level dictionary (so the infohash uses exactly those bytes).
func infoSpan(data []byte) (int, int, error) {
	i := 1 // skip the top 'd'
	for i < len(data) && data[i] != 'e' {
		k, ni, err := decode(data, i)
		if err != nil {
			return 0, 0, err
		}
		vs := ni
		_, ni2, err := decode(data, ni)
		if err != nil {
			return 0, 0, err
		}
		if ks, _ := k.(string); ks == "info" {
			return vs, ni2, nil
		}
		i = ni2
	}
	return 0, 0, fmt.Errorf("%w: missing info key", errBencode)
}

func indexByte(data []byte, b byte, from int) int {
	for i := from; i < len(data); i++ {
		if data[i] == b {
			return i
		}
	}
	return -1
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
