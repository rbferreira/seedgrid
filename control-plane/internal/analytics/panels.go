package analytics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rbferreira/seedgrid/control-plane/internal/release"
)

// keyTitle groups copies of the same content.
//
// Reuses release.Parse (ADR-020) instead of a new parser: it already yields a clean title,
// year and season/episode, and is already covered by tests. It was exactly the key used in the
// manual cleanup on 06/09/2026, which found 36 groups in 983 torrents.
//
// Resolution stays OUT of the purpose key — it is precisely what distinguishes the copies
// within a group.
//
// PART, on the other hand, is included: "The Batman Saga - Phase 1" and "- Phase 2" are different
// works, and `release.cleanTitle` cuts both down to "The Batman Saga" when applying
// the tracker's alternative title rule. Without the part number in the key, the dashboard said it could
// free 3.25 GB by deleting Phase 2 (#172).
type keyTitle struct {
	title string
	year  int
	se    string
	part  string
}

func keyOf(name string) keyTitle {
	i := release.Parse(name)
	title := strings.ToLower(strings.TrimSpace(i.Title))
	if title == "" {
		title = strings.ToLower(name)
	}
	se := ""
	if i.Season > 0 {
		se = fmt.Sprintf("S%02d", i.Season)
		if i.Episode > 0 {
			se += fmt.Sprintf("E%02d", i.Episode)
		}
	}
	return keyTitle{title: title, year: i.Year, se: se, part: strings.ToLower(i.Part)}
}

// duplicates groups by (title, year, season/episode) and returns groups with more
// than one copy, from the one that frees the most space to the one that frees the least.
func duplicates(all []Torrent) []DuplicateSet {
	groups := map[keyTitle][]Torrent{}
	for _, t := range all {
		k := keyOf(t.Name)
		groups[k] = append(groups[k], t)
	}
	var out []DuplicateSet
	for k, copies := range groups {
		if len(copies) < 2 {
			continue
		}
		sort.Slice(copies, func(i, j int) bool { return copies[i].SizeBytes > copies[j].SizeBytes })

		// Identical = same release name AND same size. It's pure waste: the same
		// thing taking up disk space on two Nodes. Different resolution is NOT this — the tracker
		// accepts both and both earn ratio, so the decision is the operator's.
		names := map[string]struct{}{}
		sizes := map[int64]struct{}{}
		for _, c := range copies {
			names[strings.TrimSpace(c.Name)] = struct{}{}
			sizes[c.SizeBytes] = struct{}{}
		}
		var reclaim int64
		for _, c := range copies[1:] { // all but the largest
			reclaim += c.SizeBytes
		}
		larger := release.Parse(copies[0].Name)
		out = append(out, DuplicateSet{
			Title:        larger.Title,
			Year:         k.year,
			SE:           k.se,
			Part:         larger.Part,
			Identical:    len(names) == 1 && len(sizes) == 1,
			Copies:       copies,
			ReclaimBytes: reclaim,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReclaimBytes > out[j].ReclaimBytes })
	return out
}

// markIdle marks the collection that takes up disk without earning anything and returns the counters.
//
// Separates "never seeded" from "never seeded AND no one else has it": in the latter the content
// died in the swarm; in the former there just wasn't demand for this Node, and there might be tomorrow.
// They are different decisions and joining them would hide this.
//
// Marks on the torrent instead of returning a truncated list (#169): the counter and the screen
// list now come from the same source, and the operator reaches the item from the number.
func markIdle(all []Torrent) IdleSummary {
	var s IdleSummary
	for i := range all {
		if all[i].Uploaded > 0 {
			continue
		}
		all[i].NeverUploaded = true
		s.NeverUploaded++
		s.NeverUploadedBytes += all[i].SizeBytes
		if all[i].NumSeeds == 0 {
			all[i].DeadSwarm = true
			s.DeadSwarm++
			s.DeadSwarmBytes += all[i].SizeBytes
		}
	}
	return s
}

// markDuplicates marks the SURPLUS copies of each group — those that would free disk if
// removed. The largest of each group is left out: it's the one intended to be kept.
func markDuplicates(all []Torrent, dups []DuplicateSet) {
	extra := make(map[string]string, len(dups))
	for _, d := range dups {
		label := d.Title
		if d.Year > 0 {
			label += fmt.Sprintf(" (%d)", d.Year)
		}
		if d.SE != "" {
			label += " " + d.SE
		}
		// Without the part in the label, "copy of: The Batman Saga" points to a title that
		// doesn't exist alone in the collection, and the operator doesn't know which copy it is.
		if d.Part != "" {
			label += " — " + d.Part
		}
		for _, c := range d.Copies[1:] {
			extra[c.ID] = label
		}
	}
	for i := range all {
		if r, ok := extra[all[i].ID]; ok {
			all[i].DuplicateExtra = true
			all[i].DuplicateOf = r
		}
	}
}

// nodeUsage cross-references usage with the recoverable per Node — the screen that answers "where
// do I get space on this Node here".
func nodeUsage(all []Torrent, dups []DuplicateSet) []NodeUsage {
	idx := map[string]*NodeUsage{}
	for _, t := range all {
		u := idx[t.NodeID]
		if u == nil {
			u = &NodeUsage{NodeID: t.NodeID, Hostname: t.Hostname, DisplayName: t.DisplayName}
			idx[t.NodeID] = u
		}
		u.Torrents++
		u.Bytes += t.SizeBytes
		if t.Uploaded == 0 {
			u.IdleBytes += t.SizeBytes
		}
	}
	// Recoverable by duplication: counts on the Node of EACH surplus copy, not on the group's
	// — that's where the disk would be freed.
	for _, d := range dups {
		for _, c := range d.Copies[1:] {
			if u := idx[c.NodeID]; u != nil {
				u.ReclaimBytes += c.SizeBytes
			}
		}
	}
	out := make([]NodeUsage, 0, len(idx))
	for _, u := range idx {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}
