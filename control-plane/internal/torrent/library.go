package torrent

import (
	"context"
	"sort"
	"strings"

	"github.com/rbferreira/seedgrid/control-plane/internal/release"
)

// AddOptions are the operator confirmations that release an add the guard would refuse.
type AddOptions struct {
	// AllowDuplicate: the same info_hash is already in the cluster and the operator wants a 2nd
	// copy (#11). The Portal no longer offers it (#37), but the API still accepts it.
	AllowDuplicate bool
	// AllowOtherVersion: the cluster already has ANOTHER version of the same work (another
	// resolution, another codec, another release) and the operator saw which ones and confirmed (#37).
	AllowOtherVersion bool
}

// Version is a version of a work that is already in the cluster — what the operator needs to see to
// decide whether they want one more: name, resolution, codec, size and where it is (#37).
type Version struct {
	Name       string `json:"name"`
	Resolution string `json:"resolution"`
	Codec      string `json:"codec"`
	SizeBytes  int64  `json:"size_bytes"`
	Node       string `json:"node"`
}

// OtherVersionsError says the cluster already has another version of the same work. The add returns
// this instead of adding when the operator hasn't confirmed yet (AllowOtherVersion).
type OtherVersionsError struct {
	Versions []Version
	// Adding is what is being added, with the same fields (no Node, which the Scheduler hasn't chosen
	// yet), so the Portal can compare them side by side.
	Adding Version
}

func (e *OtherVersionsError) Error() string {
	names := make([]string, 0, len(e.Versions))
	for _, v := range e.Versions {
		names = append(names, "«"+v.Name+"»")
	}
	return "the cluster already has another version of this title: " + strings.Join(names, ", ")
}

type libraryEntry struct {
	hash     string
	normName string
	key      release.TitleKey
	v        Version
}

// Library is the cluster's live collection with each torrent's title key already computed. One read
// serves a whole catalog page: the names are parsed once per request, not once per item.
type Library struct {
	entries []libraryEntry
}

// Library reads the cluster's live (not removed) collection.
func (s *Service) Library(ctx context.Context) (*Library, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(t.hash, ''), t.name, COALESCE(t.size_bytes, 0),
		        COALESCE(NULLIF(n.display_name, ''), NULLIF(n.hostname, ''), n.id::text)
		   FROM torrents t JOIN nodes n ON n.id = t.node_id
		  WHERE t.deleted_at IS NULL AND COALESCE(t.name, '') <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lib := &Library{}
	for rows.Next() {
		var hash, name, node string
		var size int64
		if err := rows.Scan(&hash, &name, &size, &node); err != nil {
			return nil, err
		}
		lib.entries = append(lib.entries, newLibraryEntry(strings.ToLower(hash), name, size, node))
	}
	return lib, rows.Err()
}

func newLibraryEntry(hash, name string, size int64, node string) libraryEntry {
	info := release.Parse(name)
	resolution := info.Resolution
	if resolution == "" {
		resolution = release.ParseResolution(name)
	}
	return libraryEntry{
		hash:     hash,
		normName: NormalizeName(name),
		key:      release.KeyOf(name),
		v: Version{Name: name, Resolution: resolution, Codec: info.Encoding,
			SizeBytes: size, Node: node},
	}
}

// HasFile says whether a catalog item is a FILE the cluster already has (#37).
//
// A private tracker's catalog doesn't expose the info_hash (magnet_link comes back null), and the
// catalog name isn't the torrent name ("DD+" × "DDP", words in another order): matching by name
// alone missed 23% of what the cluster had (measured on a production deployment). The identity here
// is the EXACT size in bytes, confirmed by name evidence — the same work, one of the item's files
// named like the torrent (or inside its folder), or the old name match. Size alone isn't enough: a
// false positive would hide the button of an item the cluster doesn't have. A false negative is
// cheap — the hash guard on click still catches it.
func (l *Library) HasFile(size int64, catalogName string, fileNames []string) bool {
	if l == nil || size <= 0 {
		return false
	}
	var key release.TitleKey
	keyed := false
	for _, e := range l.entries {
		if e.v.SizeBytes != size {
			continue
		}
		if !keyed {
			key, keyed = release.KeyOf(catalogName), true
		}
		if key.SameWork(e.key) || NameInCluster(catalogName, []string{e.normName}) {
			return true
		}
		for _, f := range fileNames {
			if sameFileOrFolder(NormalizeName(f), e.normName) {
				return true
			}
		}
	}
	return false
}

// sameFileOrFolder says whether the catalog file is the cluster's torrent: the same name (single-file
// torrent) or the torrent name as a prefix of the file (season folder: "Tiefsee.S01" ×
// "Tiefsee.S01E01…mkv"). It's the evidence left when the catalog writes the title in another language,
// seen in a real cluster (#37). The 8-character minimum keeps a short name from being a prefix of
// anything.
func sameFileOrFolder(file, torrentName string) bool {
	return file == torrentName || (len(torrentName) >= 8 && strings.HasPrefix(file, torrentName))
}

// NameMatch is the name-only match from #12 (NameInCluster), the fallback for a catalog item without
// a known size.
func (l *Library) NameMatch(catalogName string) bool {
	if l == nil {
		return false
	}
	names := make([]string, 0, len(l.entries))
	for _, e := range l.entries {
		names = append(names, e.normName)
	}
	return NameInCluster(catalogName, names)
}

// OtherVersions returns the versions of the same work as `name` that the cluster already has, from
// the largest to the smallest (#37). The file itself is left out: by hash, when it's known (add), or
// by size, when that's all that's known (catalog).
func (l *Library) OtherVersions(name, exceptHash string, exceptSize int64) []Version {
	if l == nil || strings.TrimSpace(name) == "" {
		return nil
	}
	key := release.KeyOf(name)
	exceptHash = strings.ToLower(exceptHash)
	var out []Version
	for _, e := range l.entries {
		if exceptHash != "" && e.hash == exceptHash {
			continue
		}
		if exceptSize > 0 && e.v.SizeBytes == exceptSize {
			continue
		}
		if key.SameWork(e.key) {
			out = append(out, e.v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SizeBytes > out[j].SizeBytes })
	return out
}
