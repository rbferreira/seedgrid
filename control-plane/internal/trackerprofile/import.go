package trackerprofile

// Import plan and application (#197). Two pure functions, no DB — the same split as the
// discovery merge (#196), so the rule is testable on its own:
//
//   - PlanImport compares the file (already validated by ParseDocument) with the current profile and says
//     what would change. NEVER writes: it is the preview.
//   - ApplyImport executes the plan. The caller computes the plan over the LOCKED state
//     (Service.Import), so the rule protecting what the operator adjusted by hand runs on the
//     server, over what is in the DB now — not over what the screen saw.
//
// The rule, in full (it is the discovery merge rule, extended to what the file brings extra):
//
//	id missing from profile          → ADD (origin imported)
//	id present, same name       → nothing on name (already there)
//	id present, different name   → RENAME only if origin is seed or imported (came from
//	                                migration or a previous import)
//	id present, origin manual or discovered → PROTECTED: keeps and says it kept — unless
//	                                the operator asks, term by term, to overwrite. Manual is
//	                                what they typed; discovered is what THEIR SITE
//	                                confirmed, and the operator's site name is worth more than a
//	                                third-party file (discovery, in turn, treats the
//	                                rename as opt-in). Unknown origin: protected (closed
//	                                failure)
//	profile term that file doesn't bring → UNTOUCHED. Import never removes: absence in
//	                                the file is not evidence (the file might be from a profile
//	                                that is incomplete, like the discovered one)
//	kind                          → file only FILLS a role that is empty; never swaps
//	                                one that already exists (the operator might have adjusted it without
//	                                the name origin changing)
//
// The origin of everything the import writes is `imported`: it came from outside, and this
// operator's site hasn't confirmed it yet.
//
// The description section (#192) has no terms or origin: it is ONE unit, and the rule is the same
// as manual, applied to the entire section —
//
//	profile without section (or with default)  → APPLIES (status new)
//	section same as file               → nothing (same)
//	section different                        → PROTECTED: keeps and says it kept — unless the
//	                                         operator asks (?overwrite=description), then REPLACES
//
// There is no "imported origin" for a section the operator might have edited by hand, so any
// config that isn't default is treated as theirs. A file that doesn't bring the section doesn't touch the
// profile's.
// The image_host section (#193) follows the same rule: it is a unit, protected when already configured,
// unless there is an explicit overwrite request.
//
// Collision: the publish panel matches the release name with the term's and chooses the first
// category of each role. A new term that shares the name (ignoring case and accent) with another
// on the axis, or becomes the first category of a role, would change the panel's choice
// without the operator deciding anything — the plan warns on every affected line.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rbferreira/seedgrid/control-plane/internal/description"
	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
)

// StatusProtected: the id exists with a different name and belongs to the operator (manual) or was confirmed
// by their site (discovered) — the import keeps it. (new, same and renamed come from the discovery
// merge.)
const StatusProtected = "protected"

// StatusReplaced: only in the description and image_host sections — the profile already had its own config
// and the operator asked to replace it with the one from the file.
const StatusReplaced = "replaced"

// overwriteDescription is the overwrite request for the description section (`?overwrite=description`).
const overwriteDescription = "description"

// overwriteImageHost is the overwrite request for the image_host section (`?overwrite=image_host`).
const overwriteImageHost = "image_host"

// yieldsToImport says if the import can rename, WITHOUT a request, a term from this origin: only what came
// from the migration or a previous import. CLOSED FAILURE — any other value is protected.
func yieldsToImport(origin string) bool {
	return origin == OriginSeed || origin == OriginImported
}

// OverwriteKey identifies a protected term whose name the operator asked to overwrite — or,
// with DescriptionKey, the entire description section.
type OverwriteKey struct {
	Axis string
	ID   int
}

// DescriptionKey is the request to overwrite the description section (#192).
var DescriptionKey = OverwriteKey{Axis: overwriteDescription}

// ImageHostKey is the request to overwrite the image_host section (#193).
var ImageHostKey = OverwriteKey{Axis: overwriteImageHost}

// ParseOverwrite reads the overwrite requests (`axis:id`, one per parameter). They are untrusted
// content like the rest of the request: known axis, id in range, limited quantity.
func ParseOverwrite(specs []string) (map[OverwriteKey]bool, error) {
	if len(specs) > MaxApplyItems {
		return nil, invalid("overwrite", "%d requested (maximum %d)", len(specs), MaxApplyItems)
	}
	out := make(map[OverwriteKey]bool, len(specs))
	for _, s := range specs {
		if s == overwriteDescription {
			out[DescriptionKey] = true
			continue
		}
		if s == overwriteImageHost {
			out[ImageHostKey] = true
			continue
		}
		axis, idText, ok := strings.Cut(s, ":")
		if !ok || !KnownAxis(axis) || len(idText) > 8 {
			return nil, invalid("overwrite", "%q invalid (use axis:id, e.g., category:2, or description, or image_host)", truncate(s, 32))
		}
		id, err := strconv.Atoi(idText)
		if err != nil {
			return nil, invalid("overwrite", "%q invalid (id is not a number)", truncate(s, 32))
		}
		if err := validateID(id); err != nil {
			return nil, invalid("overwrite", "%q: %v", truncate(s, 32), err)
		}
		out[OverwriteKey{Axis: axis, ID: id}] = true
	}
	return out, nil
}

// ImportItem is a line in the plan: a term from the file and what happens to it.
type ImportItem struct {
	Axis   string `json:"axis"`
	ID     int    `json:"id"`
	Name   string `json:"name"` // the name that the file brings
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status"`
	// Filled when the term already exists with another name (renamed/protected): what the
	// profile has today and where it came from.
	CurrentName   string `json:"current_name,omitempty"`
	CurrentOrigin string `json:"current_origin,omitempty"`
	// SetsKind: the profile has no role for this term and the file brings one — it will be filled.
	SetsKind bool `json:"sets_kind,omitempty"`
	// OverwritesProtected: the operator asked to overwrite a protected name (manual or
	// confirmed by their site).
	OverwritesProtected bool `json:"overwrites_protected,omitempty"`
	// Warnings: collisions of this term with what the profile already has (see top of file).
	Warnings []string `json:"warnings,omitempty"`
}

// ImportCounts summarizes the plan.
type ImportCounts struct {
	New       int `json:"new"`
	Renamed   int `json:"renamed"`
	Same      int `json:"same"`
	Protected int `json:"protected"`
	// KindSet: terms that already exist and get a role they didn't have.
	KindSet int `json:"kind_set"`
	// LocalOnly: profile terms that the file doesn't bring (they stay as they are).
	LocalOnly int `json:"local_only"`
}

// DescriptionItem is the plan line for the description section (#192): what the file brings
// (already normalized) and what happens to the profile's section.
type DescriptionItem struct {
	// Status: new (the profile didn't have its own config), same, replaced (the operator asked to
	// replace theirs) or protected (theirs was kept).
	Status string             `json:"status"`
	Config description.Config `json:"config"`
	// Current: the config the profile has today — filled when it's different from the file's
	// (protected and replaced), so the screen can show what would be lost.
	Current *description.Config `json:"current,omitempty"`
	// OverwritesProtected: the operator asked to overwrite the config they had.
	OverwritesProtected bool `json:"overwrites_protected,omitempty"`
}

// ImageHostItem is the plan line for the image_host section (#193): what the file brings
// (already validated) and what happens to the profile's section.
type ImageHostItem struct {
	// Status: new (the profile had no custom config), same, replaced (the operator asked to
	// replace it) or protected (it was kept).
	Status string         `json:"status"`
	Config imagehost.Spec `json:"config"`
	// Current: the config the profile has today — filled in when it differs from the file's
	// (protected and replaced), so the screen shows what would be lost.
	Current *imagehost.Spec `json:"current,omitempty"`
	// OverwritesProtected: the operator asked to overwrite the existing config.
	OverwritesProtected bool `json:"overwrites_protected,omitempty"`
}

// ImportPlan is the preview: what the file is, what would change, and what deserves a warning.
type ImportPlan struct {
	Site          Site                    `json:"site"`
	Family        string                  `json:"family"`
	SchemaVersion int                     `json:"schema_version"`
	Items         map[string][]ImportItem `json:"items"`
	Counts        ImportCounts            `json:"counts"`
	// Description only exists when the file brings the section (omitted from JSON otherwise).
	Description *DescriptionItem `json:"description,omitempty"`
	// ImageHost only exists when the file brings the section (omitted from JSON otherwise).
	ImageHost *ImageHostItem `json:"image_host,omitempty"`
	Warnings  []string       `json:"warnings"`
}

// Changes indicates whether applying the plan would change anything in the profile.
func (p ImportPlan) Changes() bool {
	return p.Counts.New+p.Counts.Renamed+p.Counts.KindSet > 0 || p.descriptionChanges() || p.imageHostChanges()
}

func (p ImportPlan) descriptionChanges() bool {
	return p.Description != nil && (p.Description.Status == StatusNew || p.Description.Status == StatusReplaced)
}

func (p ImportPlan) imageHostChanges() bool {
	return p.ImageHost != nil && (p.ImageHost.Status == StatusNew || p.ImageHost.Status == StatusReplaced)
}

// PlanImport compares the file with the current profile. `target` is the target tracker's
// identification (just to warn when the file is from another site). Errors when applying
// would exceed the term limit of an axis.
func PlanImport(current Profile, doc Document, target Site, overwrite map[OverwriteKey]bool) (ImportPlan, error) {
	cur := normalize(current.Taxonomy)
	plan := ImportPlan{
		Site:          doc.Site,
		Family:        doc.Family,
		SchemaVersion: doc.SchemaVersion,
		Items:         make(map[string][]ImportItem, len(Axes)),
		Warnings:      []string{},
	}

	total, kindConflicts := 0, 0
	matched := map[OverwriteKey]bool{}
	for _, axis := range Axes {
		plan.Items[axis] = []ImportItem{}
		terms := doc.Taxonomy[axis]
		if len(terms) == 0 {
			continue
		}
		total += len(terms)

		byID := make(map[int]Term, len(cur[axis]))
		for _, t := range cur[axis] {
			byID[t.ID] = t
		}
		inFile := make(map[int]bool, len(terms))
		newInAxis := 0

		for _, t := range terms {
			inFile[t.ID] = true
			item := ImportItem{Axis: axis, ID: t.ID, Name: t.Name, Kind: t.Kind}
			have, exists := byID[t.ID]
			switch {
			case !exists:
				item.Status = StatusNew
				newInAxis++
				plan.Counts.New++
			default:
				fillKind := have.Kind == "" && t.Kind != ""
				key := OverwriteKey{Axis: axis, ID: t.ID}
				switch {
				case have.Name == t.Name:
					item.Status = StatusSame
					plan.Counts.Same++
				case !yieldsToImport(have.Origin) && !overwrite[key]:
					item.Status = StatusProtected
					item.CurrentName, item.CurrentOrigin = have.Name, have.Origin
					plan.Counts.Protected++
					plan.Items[axis] = append(plan.Items[axis], item)
					continue // protected: not even the role is touched
				default:
					item.Status = StatusRenamed
					item.CurrentName, item.CurrentOrigin = have.Name, have.Origin
					if !yieldsToImport(have.Origin) {
						item.OverwritesProtected = true
						matched[key] = true
					}
					plan.Counts.Renamed++
				}
				if fillKind {
					item.SetsKind = true
					plan.Counts.KindSet++
				} else if have.Kind != "" && t.Kind != "" && have.Kind != t.Kind {
					kindConflicts++
				}
			}
			plan.Items[axis] = append(plan.Items[axis], item)
		}

		if n := len(cur[axis]) + newInAxis; n > MaxTermsPerAxis {
			return ImportPlan{}, invalid("taxonomy."+axis, "importing would leave the axis with %d terms (maximum %d)", n, MaxTermsPerAxis)
		}
		for _, t := range cur[axis] {
			if !inFile[t.ID] {
				plan.Counts.LocalOnly++
			}
		}
	}

	if doc.Description != nil {
		plan.Description = planDescription(current.Description, *doc.Description, overwrite, matched)
		if plan.Description.Status == StatusProtected {
			plan.Warnings = append(plan.Warnings,
				"The file contains a description composition different from the one you configured for this tracker; yours was kept. "+
					"To replace it with the one from the file, request overwriting the description.")
		}
	}
	if doc.ImageHost != nil {
		plan.ImageHost = planImageHost(current.ImageHost, *doc.ImageHost, overwrite, matched)
		if plan.ImageHost.Status == StatusProtected {
			plan.Warnings = append(plan.Warnings,
				"The file contains an image host configuration different from the one you configured for this tracker; yours was kept. "+
					"To replace it with the one from the file, request overwriting the image host.")
		}
	}
	if total == 0 && doc.Description == nil && doc.ImageHost == nil {
		plan.Warnings = append(plan.Warnings, "The file contains no terms: there is nothing to import.")
	}
	// The ids are from the SITE: importing another's profile publishes in the wrong category, silently. The
	// comparison is by host, and when it's not possible — the file doesn't bring a host, or the tracker
	// doesn't have a valid host — the warning is also issued: a file without a host cannot disable the
	// check just by omitting the field.
	switch {
	case doc.Site.Host == "":
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"The file («%s») does not carry the site host, so there is no way to confirm it is the profile of this tracker's site. "+
				"The ids are from the site: check before applying — a wrong id publishes to the wrong category.", doc.Site.Name))
	case target.Host == "":
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"This tracker does not have a valid host to compare with the one in the file (%s). The ids are from the site: check that the "+
				"profile is from the same site before applying.", doc.Site.Host))
	case target.Host != doc.Site.Host:
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"The file is from the site «%s» (%s) and this tracker is «%s» (%s). The ids are from the site: import the profile from another site "+
				"only if you are sure that the ids match — a wrong id publishes to the wrong category.",
			doc.Site.Name, doc.Site.Host, target.Name, target.Host))
	}
	if flagged := annotateCollisions(cur, &plan); flagged > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"%d term(s) from the file collide with what your profile already has — duplicate name or role that would change the category "+
				"default of the publication panel. See the warning on each line.", flagged))
	}
	if kindConflicts > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"%d category(ies) have a different role in the file than the one you already set; yours was kept.", kindConflicts))
	}
	if unmatched := len(overwrite) - len(matched); unmatched > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"%d override request(s) do not match any protected term (or description or image host) in the file; they were ignored.", unmatched))
	}
	return plan, nil
}

// planDescription decides what happens to the profile's description section versus the file's
// (already validated). It errs on the side of NOT touching what the operator configured.
func planDescription(cur *description.Config, file description.Config, overwrite, matched map[OverwriteKey]bool) *DescriptionItem {
	file = file.Normalized()
	item := &DescriptionItem{Config: file}
	def := description.DefaultConfig()
	switch {
	case cur == nil || cur.Equal(def):
		// Nothing the operator has configured to protect: the default is "no section".
		if file.Equal(def) {
			item.Status = StatusSame
		} else {
			item.Status = StatusNew
		}
	case cur.Equal(file):
		item.Status = StatusSame
	default:
		c := cur.Normalized()
		item.Current = &c
		if overwrite[DescriptionKey] {
			item.Status = StatusReplaced
			item.OverwritesProtected = true
			matched[DescriptionKey] = true
		} else {
			item.Status = StatusProtected
		}
	}
	return item
}

// planImageHost decides what happens to the profile's image_host section versus the file's
// (already validated). It errs on the side of NOT touching what the operator configured.
func planImageHost(cur *imagehost.Spec, file imagehost.Spec, overwrite, matched map[OverwriteKey]bool) *ImageHostItem {
	item := &ImageHostItem{Config: file}
	switch {
	case cur == nil:
		// Nothing the operator has configured to protect: no section means "no custom host".
		item.Status = StatusNew
	case *cur == file:
		item.Status = StatusSame
	default:
		c := *cur
		item.Current = &c
		if overwrite[ImageHostKey] {
			item.Status = StatusReplaced
			item.OverwritesProtected = true
			matched[ImageHostKey] = true
		} else {
			item.Status = StatusProtected
		}
	}
	return item
}

var kindLabel = map[string]string{KindMovie: "Movie", KindTV: "Series", KindGame: "Game"}

// annotateCollisions checks the state each axis WOULD BE in if the plan were applied and
// marks, for each new or renamed term, what it would start sharing with another:
//
//   - the name — equal, ignoring case and accent (NormalizeLabel), to another id in the axis, in the profile
//     or among what the plan changes. The panel matches by name and, with two ids for the same label, it
//     has no way to choose: a new term with the name of one the operator already has would obscure it;
//   - the role — when the term becomes the FIRST category (by id) of a role that already had
//     an owner. Multiple terms with the same role are normal (series and anime); what changes the panel's
//     choice is the role's owner changing, and it only warns in this case.
//
// Returns how many terms were marked.
func annotateCollisions(cur Taxonomy, plan *ImportPlan) int {
	flagged := 0
	for _, axis := range Axes {
		items := plan.Items[axis]
		if len(items) == 0 {
			continue
		}
		final := make(map[int]Term, len(cur[axis])+len(items))
		for _, t := range cur[axis] {
			final[t.ID] = t
		}
		for _, it := range items {
			t := final[it.ID]
			switch it.Status {
			case StatusNew:
				t = Term{ID: it.ID, Name: it.Name, Kind: it.Kind}
			case StatusRenamed:
				t.Name = it.Name
				if it.SetsKind {
					t.Kind = it.Kind
				}
			case StatusSame:
				if it.SetsKind {
					t.Kind = it.Kind
				}
			default:
				continue
			}
			final[it.ID] = t
		}

		byName := make(map[string][]int, len(final))
		for id, t := range final {
			k := NormalizeLabel(t.Name)
			byName[k] = append(byName[k], id)
		}
		warned := make(map[int]bool)
		for i := range items {
			it := &items[i]
			if it.Status != StatusNew && it.Status != StatusRenamed {
				continue
			}
			ids := append([]int(nil), byName[NormalizeLabel(it.Name)]...)
			sort.Ints(ids)
			for _, other := range ids {
				if other != it.ID {
					it.Warnings = append(it.Warnings, fmt.Sprintf(
						"the name is the same, ignoring case and accents, as that of id %d «%s»: with two ids for the same name the publication panel "+
							"does not pre-fill", other, final[other].Name))
					warned[it.ID] = true
					break
				}
			}
		}

		if axis == AxisCategory {
			before, after := firstByKind(cur[axis]), firstByKindOf(final)
			for _, kind := range []string{KindMovie, KindTV, KindGame} {
				was, had := before[kind]
				now := after[kind]
				if !had || was == now {
					continue
				}
				for i := range items {
					if items[i].ID == now {
						items[i].Warnings = append(items[i].Warnings, fmt.Sprintf(
							"is now the first category with the role «%s» (it used to be id %d «%s»): the publication panel "+
								"pre-fills the first by id", kindLabel[kind], was, final[was].Name))
						warned[now] = true
					}
				}
			}
		}
		flagged += len(warned)
	}
	return flagged
}

// firstByKind maps each role to the lowest id that has it.
func firstByKind(terms []Term) map[string]int {
	m := map[string]int{}
	for _, t := range terms {
		if t.Kind == "" {
			continue
		}
		if id, ok := m[t.Kind]; !ok || t.ID < id {
			m[t.Kind] = t.ID
		}
	}
	return m
}

func firstByKindOf(terms map[int]Term) map[string]int {
	list := make([]Term, 0, len(terms))
	for _, t := range terms {
		list = append(list, t)
	}
	return firstByKind(list)
}

// ApplyImportDescription executes the plan on the description section and returns the new one (nil = no
// section). Pure: it doesn't touch `current`. Only new and replaced write; same, protected and the absence
// of the section in the file return the section as it was.
func ApplyImportDescription(current *description.Config, plan ImportPlan) *description.Config {
	if !plan.descriptionChanges() {
		return current
	}
	c := plan.Description.Config.Normalized()
	return &c
}

// ApplyImportImageHost executes the plan on the image_host section and returns the new one (nil = no
// section). Pure: it doesn't touch `current`. Only new and replaced write; same, protected and the absence
// of the section in the file return the section as it was.
func ApplyImportImageHost(current *imagehost.Spec, plan ImportPlan) *imagehost.Spec {
	if !plan.imageHostChanges() {
		return current
	}
	c := plan.ImageHost.Config
	return &c
}

// ApplyImport executes the plan on the taxonomy and returns the new one. Pure: it doesn't touch
// `current`. The plan must have been calculated on THIS taxonomy (Service.Import does it
// inside the transaction, on the locked state).
func ApplyImport(current Taxonomy, plan ImportPlan) Taxonomy {
	out := normalize(current)
	for _, axis := range Axes {
		pos := make(map[int]int, len(out[axis]))
		for i, t := range out[axis] {
			pos[t.ID] = i
		}
		for _, it := range plan.Items[axis] {
			switch it.Status {
			case StatusNew:
				out[axis] = append(out[axis], Term{ID: it.ID, Name: it.Name, Kind: it.Kind, Origin: OriginImported})
			case StatusRenamed:
				if i, ok := pos[it.ID]; ok {
					out[axis][i].Name = it.Name
					out[axis][i].Origin = OriginImported
					if it.SetsKind {
						out[axis][i].Kind = it.Kind
					}
				}
			case StatusSame:
				// Only the role: the name was already this, and the origin doesn't change (changing only the kind doesn't
				// change the origin — same rule as manual editing).
				if i, ok := pos[it.ID]; ok && it.SetsKind {
					out[axis][i].Kind = it.Kind
				}
			}
		}
	}
	return normalize(out)
}
