package rtorrent

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// stripKeys removes keys from a bencoded dictionary, keeping every other byte as it was: the info
// dictionary, and with it the infohash, must come out identical (ADR-045 §6).
func stripKeys(data []byte, keys ...string) ([]byte, error) {
	if len(data) == 0 || data[0] != 'd' {
		return nil, errors.New("rtorrent: .torrent is not a bencoded dictionary")
	}
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	var out bytes.Buffer
	out.WriteByte('d')
	i := 1
	for {
		if i >= len(data) {
			return nil, errors.New("rtorrent: .torrent ends inside the dictionary")
		}
		if data[i] == 'e' {
			out.WriteByte('e')
			if i+1 != len(data) {
				return nil, errors.New("rtorrent: bytes after the .torrent's dictionary")
			}
			return out.Bytes(), nil
		}
		keyEnd, err := skipValue(data, i)
		if err != nil {
			return nil, err
		}
		if data[i] < '0' || data[i] > '9' {
			return nil, fmt.Errorf("rtorrent: .torrent key at byte %d is not a string", i)
		}
		key := data[i:keyEnd]
		key = key[bytes.IndexByte(key, ':')+1:]
		valEnd, err := skipValue(data, keyEnd)
		if err != nil {
			return nil, err
		}
		if !drop[string(key)] {
			out.Write(data[i:valEnd])
		}
		i = valEnd
	}
}

// skipValue returns where the bencoded value starting at i ends.
func skipValue(data []byte, i int) (int, error) {
	if i >= len(data) {
		return 0, errors.New("rtorrent: truncated bencode")
	}
	switch c := data[i]; {
	case c == 'i':
		end := bytes.IndexByte(data[i:], 'e')
		if end < 0 {
			return 0, errors.New("rtorrent: unterminated integer")
		}
		return i + end + 1, nil
	case c == 'l' || c == 'd':
		j := i + 1
		for {
			if j >= len(data) {
				return 0, errors.New("rtorrent: unterminated list or dictionary")
			}
			if data[j] == 'e' {
				return j + 1, nil
			}
			var err error
			if j, err = skipValue(data, j); err != nil {
				return 0, err
			}
		}
	case c >= '0' && c <= '9':
		colon := bytes.IndexByte(data[i:], ':')
		if colon < 0 {
			return 0, errors.New("rtorrent: string without length")
		}
		n, err := strconv.Atoi(string(data[i : i+colon]))
		if err != nil || n < 0 {
			return 0, errors.New("rtorrent: bad string length")
		}
		end := i + colon + 1 + n
		if end > len(data) {
			return 0, errors.New("rtorrent: truncated string")
		}
		return end, nil
	default:
		return 0, fmt.Errorf("rtorrent: bad bencode at byte %d", i)
	}
}
