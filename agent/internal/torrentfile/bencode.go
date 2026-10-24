package torrentfile

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // G505: the BitTorrent v1 info hash is SHA-1, by the protocol
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

// InfoHash is the v1 info hash of a .torrent: the SHA-1 of its info dictionary as it is in the
// file, in lowercase hex.
func InfoHash(data []byte) (string, error) {
	if len(data) == 0 || data[0] != 'd' {
		return "", errors.New("torrentfile: not a bencoded dictionary")
	}
	i := 1
	for i < len(data) && data[i] != 'e' {
		keyEnd, err := skipValue(data, i)
		if err != nil {
			return "", err
		}
		valEnd, err := skipValue(data, keyEnd)
		if err != nil {
			return "", err
		}
		if string(data[i:keyEnd]) == "4:info" {
			sum := sha1.Sum(data[keyEnd:valEnd]) //nolint:gosec // G401: the BitTorrent v1 info hash is SHA-1
			return hex.EncodeToString(sum[:]), nil
		}
		i = valEnd
	}
	return "", errors.New("torrentfile: no info dictionary")
}

// StripKeys removes keys from a bencoded dictionary, keeping every other byte as it was: the info
// dictionary, and with it the infohash, must come out identical (ADR-045 §6).
func StripKeys(data []byte, keys ...string) ([]byte, error) {
	if len(data) == 0 || data[0] != 'd' {
		return nil, errors.New("torrentfile: .torrent is not a bencoded dictionary")
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
			return nil, errors.New("torrentfile: .torrent ends inside the dictionary")
		}
		if data[i] == 'e' {
			out.WriteByte('e')
			if i+1 != len(data) {
				return nil, errors.New("torrentfile: bytes after the .torrent's dictionary")
			}
			return out.Bytes(), nil
		}
		keyEnd, err := skipValue(data, i)
		if err != nil {
			return nil, err
		}
		if data[i] < '0' || data[i] > '9' {
			return nil, fmt.Errorf("torrentfile: .torrent key at byte %d is not a string", i)
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
		return 0, errors.New("torrentfile: truncated bencode")
	}
	switch c := data[i]; {
	case c == 'i':
		end := bytes.IndexByte(data[i:], 'e')
		if end < 0 {
			return 0, errors.New("torrentfile: unterminated integer")
		}
		return i + end + 1, nil
	case c == 'l' || c == 'd':
		j := i + 1
		for {
			if j >= len(data) {
				return 0, errors.New("torrentfile: unterminated list or dictionary")
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
			return 0, errors.New("torrentfile: string without length")
		}
		n, err := strconv.Atoi(string(data[i : i+colon]))
		if err != nil || n < 0 {
			return 0, errors.New("torrentfile: bad string length")
		}
		end := i + colon + 1 + n
		if end > len(data) {
			return 0, errors.New("torrentfile: truncated string")
		}
		return end, nil
	default:
		return 0, fmt.Errorf("torrentfile: bad bencode at byte %d", i)
	}
}
