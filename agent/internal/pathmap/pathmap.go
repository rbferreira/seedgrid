// Package pathmap translates paths between the Torrent Engine and the host (ADR-043).
//
// The Agent runs on the host and reads torrent data directly (migration, screenshots,
// MediaInfo). When the Engine runs in a container, the paths it reports are the container's
// (`/downloads/Movie`), not the host's (`/mnt/user/torrents/Movie`), and a path the Agent hands
// to the Engine (a migration destination, a publishing source) must be the container's. A Map is
// the list of bind mounts between the two, `engine path=host path`. The Nodes SeedGrid installs
// mount `/data` at `/data` and need no map: an empty Map returns every path unchanged.
package pathmap

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// pair is one bind mount: Engine is the path inside the Engine, Host the same directory on the host.
type pair struct {
	Engine string
	Host   string
}

// Map translates paths between the Engine and the host. The zero value is the identity.
type Map struct {
	pairs []pair
}

// Parse reads SEEDGRID_ENGINE_PATH_MAP: pairs `engine path=host path` separated by `;`, for
// example `/downloads=/mnt/user/torrents;/movies=/mnt/user/media/movies`. Both sides must be
// absolute. Empty text is the identity map.
func Parse(text string) (Map, error) {
	var m Map
	for _, item := range strings.Split(text, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		engine, host, ok := strings.Cut(item, "=")
		engine, host = strings.TrimSpace(engine), strings.TrimSpace(host)
		if !ok || engine == "" || host == "" {
			return Map{}, fmt.Errorf("path map entry %q: want <engine path>=<host path>", item)
		}
		if !path.IsAbs(engine) || !path.IsAbs(host) {
			return Map{}, fmt.Errorf("path map entry %q: both paths must be absolute", item)
		}
		m.pairs = append(m.pairs, pair{Engine: path.Clean(engine), Host: path.Clean(host)})
	}
	return m, nil
}

// ToHost translates a path the Engine reports into the host path. A path under no mapped
// directory is returned unchanged: on a Node without a container they are the same.
func (m Map) ToHost(p string) string {
	return translate(m.pairs, p, func(x pair) (string, string) { return x.Engine, x.Host })
}

// ToEngine translates a host path into the path the Engine sees.
func (m Map) ToEngine(p string) string {
	return translate(m.pairs, p, func(x pair) (string, string) { return x.Host, x.Engine })
}

// HostRoots lists the host side of every pair: the directories the Engine can see, where the
// Agent may serve torrent data from.
func (m Map) HostRoots() []string {
	roots := make([]string, 0, len(m.pairs))
	for _, x := range m.pairs {
		roots = append(roots, x.Host)
	}
	return roots
}

// translate rewrites the prefix `from` of p as `to`, choosing the longest matching `from`: with
// `/data` and `/data/movies` mapped to different places, `/data/movies/x` follows the second.
// A prefix only matches whole components (`/data` does not match `/database`).
func translate(pairs []pair, p string, sides func(pair) (from, to string)) string {
	if p == "" || len(pairs) == 0 {
		return p
	}
	clean := path.Clean(p)
	candidates := make([]pair, len(pairs))
	copy(candidates, pairs)
	sort.SliceStable(candidates, func(i, j int) bool {
		fi, _ := sides(candidates[i])
		fj, _ := sides(candidates[j])
		return len(fi) > len(fj)
	})
	for _, x := range candidates {
		from, to := sides(x)
		if clean == from {
			return to
		}
		prefix := from
		if prefix != "/" {
			prefix += "/"
		}
		if strings.HasPrefix(clean, prefix) {
			return path.Join(to, strings.TrimPrefix(clean, prefix))
		}
	}
	return p
}
