package trackerprofile

import (
	"fmt"
	"sort"
	"strings"
)

// This part is the coexistence rule between discovery (#196) and what the operator
// adjusted by hand. Two pure functions, no database, so the rule is testable on its own:
//
//   - Propose compares what the catalog showed with the current profile and says what would change.
//     It NEVER saves: discovery is proposed, the operator applies.
//   - Merge applies what the operator accepted. It is here, on the server, that the guarantee of
//     "does not overwrite manual" lives — checking only in the Portal wouldn't be enough, because the state
//     might have changed between proposing and applying.
//
// The rule, in full:
//
//	id missing from profile       → ADDS (origin discovered)
//	id present, same name         → nothing (already there)
//	id present, different name    → RENAMES, but only if origin is NOT manual
//	id present, manual origin     → PROTECTED: ignores and says it ignored (and the same applies to
//	                                an origin this code doesn't know: closed failure)
//	profile term catalog didn't show → UNTOUCHED. Absence is not evidence: the
//	                                catalog only shows what someone posted.
//
// Discovery never removes anything and never touches kind.

// Status of a proposal item.
const (
	StatusNew     = "new"     // id that the profile does not have
	StatusSame    = "same"    // already there, with the same name
	StatusRenamed = "renamed" // already there, but the site uses another name
)

// Observed is a pair seen in the catalog (comes from discovery, therefore not trusted).
type Observed struct {
	ID   int
	Name string
	Seen int
}

// ProposalItem is a line in the proposal: what the site showed and what that means
// compared to the current profile.
type ProposalItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"` // the name the site uses
	Seen   int    `json:"seen"` // how many times it appeared in what was scanned
	Status string `json:"status"`
	// Filled when Status == renamed: what the profile has today and where it came from. With
	// CurrentOrigin == manual, applying would do nothing (Protected).
	CurrentName   string `json:"current_name,omitempty"`
	CurrentOrigin string `json:"current_origin,omitempty"`
	Protected     bool   `json:"protected,omitempty"`
}

// Proposal is the proposal per axis, plus what was discarded and what deserves a warning.
type Proposal struct {
	Items map[string][]ProposalItem `json:"items"`
	// Rejected counts the catalog pairs that didn't pass validation (name with control
	// character, id out of range…). The catalog is third-party: we don't trust it.
	Rejected int      `json:"rejected"`
	Warnings []string `json:"warnings,omitempty"`
}

// Propose confronts the observed with the current profile. Invalid pairs are discarded and
// counted; the same id seen with two names keeps the most frequent (tie: the first
// in alphabetical order, so the response is deterministic) and generates a warning.
func Propose(current Taxonomy, observed map[string][]Observed) Proposal {
	cur := normalize(current)
	p := Proposal{Items: make(map[string][]ProposalItem, len(Axes))}
	for _, axis := range Axes {
		p.Items[axis] = []ProposalItem{}
		byID := make(map[int]Term, len(cur[axis]))
		for _, t := range cur[axis] {
			byID[t.ID] = t
		}

		// Groups by id, only with what is valid.
		cands := map[int][]Observed{}
		for _, o := range observed[axis] {
			name, err := CleanName(o.Name)
			if err != nil || validateID(o.ID) != nil || o.Seen < 0 {
				p.Rejected++
				continue
			}
			cands[o.ID] = append(cands[o.ID], Observed{ID: o.ID, Name: name, Seen: o.Seen})
		}
		ids := make([]int, 0, len(cands))
		for id := range cands {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		for _, id := range ids {
			group := cands[id]
			sort.Slice(group, func(i, j int) bool {
				if group[i].Seen != group[j].Seen {
					return group[i].Seen > group[j].Seen
				}
				return group[i].Name < group[j].Name
			})
			best := group[0]
			if len(group) > 1 {
				names := make([]string, len(group))
				for i, g := range group {
					names[i] = g.Name
				}
				p.Warnings = append(p.Warnings, fmt.Sprintf(
					"%s: id %d appeared with more than one name (%s); the most frequent was used",
					axis, id, strings.Join(names, ", ")))
			}
			// Seen for the item sums the names of the same id: it's how many times the id appeared.
			total := 0
			for _, g := range group {
				total += g.Seen
			}
			item := ProposalItem{ID: id, Name: best.Name, Seen: total}
			switch have, ok := byID[id]; {
			case !ok:
				item.Status = StatusNew
			case have.Name == best.Name:
				item.Status = StatusSame
			default:
				item.Status = StatusRenamed
				item.CurrentName = have.Name
				item.CurrentOrigin = have.Origin
				item.Protected = !yieldsToDiscovery(have.Origin)
			}
			p.Items[axis] = append(p.Items[axis], item)
		}
	}
	return p
}

// ApplyItem is a term the operator accepted from the proposal. Origin says if the name is what the
// site showed (discovered) or if the operator edited it before applying (manual); empty
// counts as discovered. seed is not accepted: only the migration seeds.
type ApplyItem struct {
	Axis   string `json:"axis"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Origin string `json:"origin,omitempty"`
}

// Actions and reasons for the Merge result.
const (
	ActionAdded   = "added"
	ActionRenamed = "renamed"

	ReasonSame      = "same"      // already there, with this name
	ReasonProtected = "protected" // the term was adjusted by hand: discovery does not overwrite
)

// Applied is what Merge changed.
type Applied struct {
	Axis   string `json:"axis"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

// Skipped is what Merge left as it was, and why.
type Skipped struct {
	Axis   string `json:"axis"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// MaxApplyItems limits an application: three full axes.
const MaxApplyItems = 3 * MaxTermsPerAxis

// ValidateApplyItems checks the items of an application (untrusted content: comes from the
// request body) and returns them clean. Any invalid item rejects the entire
// request — applying "half" of something that came with garbage would hide the problem.
func ValidateApplyItems(items []ApplyItem) ([]ApplyItem, error) {
	if len(items) == 0 {
		return nil, invalid("items", "no items to apply")
	}
	if len(items) > MaxApplyItems {
		return nil, invalid("items", "%d items (maximum %d)", len(items), MaxApplyItems)
	}
	type key struct {
		axis string
		id   int
	}
	seen := make(map[key]bool, len(items))
	out := make([]ApplyItem, 0, len(items))
	for i, it := range items {
		path := fmt.Sprintf("items[%d]", i)
		if !KnownAxis(it.Axis) {
			return nil, invalid(path+".axis", "unknown axis %q", truncate(it.Axis, 32))
		}
		if err := validateID(it.ID); err != nil {
			return nil, invalid(path+".id", "%v", err)
		}
		name, err := CleanName(it.Name)
		if err != nil {
			return nil, invalid(path+".name", "%v", err)
		}
		origin := it.Origin
		switch origin {
		case "":
			origin = OriginDiscovered
		case OriginDiscovered, OriginManual:
		default:
			return nil, invalid(path+".origin", "invalid origin %q (discovered or manual)", truncate(origin, 32))
		}
		k := key{it.Axis, it.ID}
		if seen[k] {
			return nil, invalid(path, "duplicate item (%s id %d)", it.Axis, it.ID)
		}
		seen[k] = true
		out = append(out, ApplyItem{Axis: it.Axis, ID: it.ID, Name: name, Origin: origin})
	}
	return out, nil
}

// Merge applies the accepted items over the current taxonomy according to the rule at the top of this
// file. Receives ALREADY validated items (ValidateApplyItems). Returns the new taxonomy and the
// report of what changed and what was ignored.
func Merge(current Taxonomy, items []ApplyItem) (Taxonomy, []Applied, []Skipped, error) {
	out := normalize(current)
	var applied []Applied
	var skipped []Skipped
	for _, it := range items {
		terms := out[it.Axis]
		idx := -1
		for i, t := range terms {
			if t.ID == it.ID {
				idx = i
				break
			}
		}
		switch {
		case idx < 0:
			if len(terms) >= MaxTermsPerAxis {
				return nil, nil, nil, invalid("taxonomy."+it.Axis, "the axis already has %d terms (maximum %d)", len(terms), MaxTermsPerAxis)
			}
			out[it.Axis] = append(terms, Term{ID: it.ID, Name: it.Name, Origin: it.Origin})
			applied = append(applied, Applied{Axis: it.Axis, ID: it.ID, Name: it.Name, Action: ActionAdded})
		case terms[idx].Name == it.Name:
			skipped = append(skipped, Skipped{Axis: it.Axis, ID: it.ID, Name: it.Name, Reason: ReasonSame})
		case !yieldsToDiscovery(terms[idx].Origin):
			skipped = append(skipped, Skipped{Axis: it.Axis, ID: it.ID, Name: it.Name, Reason: ReasonProtected})
		default:
			// kind stays as is: discovery knows nothing about it.
			terms[idx].Name = it.Name
			terms[idx].Origin = it.Origin
			applied = append(applied, Applied{Axis: it.Axis, ID: it.ID, Name: it.Name, Action: ActionRenamed})
		}
	}
	return normalize(out), applied, skipped, nil
}
