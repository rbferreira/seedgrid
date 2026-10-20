package analytics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rbferreira/seedgrid/control-plane/internal/release"
)

// duplicates groups by work (release.TitleKey: title, year, season/episode and part) and returns
// the groups with more than one copy, from the one that frees the most space to the one that
// frees the least.
//
// The key is the same one as the other-version warning on add (#37), so Analytics and add never
// disagree about what "the same title" is. It groups by the whole key, year included: the
// missing-year tolerance of SameWork isn't transitive and can't be a map key — and a panel that
// suggests deleting copies should err on the side of not grouping.
func duplicates(all []Torrent) []DuplicateSet {
	groups := map[release.TitleKey][]Torrent{}
	for _, t := range all {
		k := release.KeyOf(t.Name)
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
			Year:         k.Year,
			SE:           k.SE,
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
