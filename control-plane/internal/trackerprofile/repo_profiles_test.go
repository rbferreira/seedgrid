package trackerprofile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoProfilesDir is the directory of contributed profiles at the repository root (#36).
const repoProfilesDir = "../../../profiles"

// TestRepoProfiles validates each versioned profile in the repository with the SAME import rule
// by the Portal: if a file passes here, an operator can import it.
//
// Besides what ParseDocument already guarantees (format, version, family, taxonomy, and no
// unknown fields — which blocks credentials by design), the directory has three rules:
//   - the file goes in profiles/<family>/, and the directory's family is the document's;
//   - the filename is what export suggests for that site (without the prefix): two
//     profiles for the same site cannot coexist with different names;
//   - the content is in the canonical form of export — it's the file the Portal generated, not a
//     manual edit that the next export would undo.
func TestRepoProfiles(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repoProfilesDir, "*", "*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if _, statErr := os.Stat(repoProfilesDir); statErr != nil {
		t.Skipf("profiles directory missing (module outside the repository?): %v", statErr)
	}
	if len(files) == 0 {
		t.Fatal("profiles/ exists but contains no profiles — did the example disappear?")
	}
	for _, f := range files {
		rel := filepath.ToSlash(strings.TrimPrefix(filepath.ToSlash(f), repoProfilesDir+"/"))
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(f) //nolint:gosec // G304: path from glob in the repository directory
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			doc, err := ParseDocument(raw)
			if err != nil {
				t.Fatalf("the import would reject this file: %v", err)
			}
			dir := filepath.Base(filepath.Dir(f))
			if doc.Family != dir {
				t.Errorf("document family is %q, but the file is in profiles/%s/", doc.Family, dir)
			}
			want := strings.TrimPrefix(doc.Filename(), "seedgrid-profile-"+doc.Family+"-")
			if got := filepath.Base(f); got != want {
				t.Errorf("file name is %q, want %q (derived from the site host)", got, want)
			}
			canon, err := doc.Marshal()
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !bytes.Equal(bytes.TrimSpace(raw), bytes.TrimSpace(canon)) {
				t.Errorf("the file is not in export form — reexport via the Portal instead of editing by hand.\n--- want ---\n%s", canon)
			}
		})
	}
}

// TestRepoProfileTemplate: profiles/TEMPLATE.json is the starting point for anyone writing a
// profile without SeedGrid running (profiles/README.md). It must remain importable, with
// all sections, and in export's canonical form — whoever copies the template and only changes values
// passes TestRepoProfiles.
func TestRepoProfileTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoProfilesDir, "TEMPLATE.json"))
	if err != nil {
		t.Skipf("template missing (module outside the repository?): %v", err)
	}
	doc, err := ParseDocument(raw)
	if err != nil {
		t.Fatalf("import rejected the template: %v", err)
	}
	if doc.Description == nil || doc.ImageHost == nil {
		t.Error("the template must show the optional sections (description and image_host)")
	}
	for _, axis := range Axes {
		if len(doc.Taxonomy[axis]) == 0 {
			t.Errorf("the template has no example for axis %s", axis)
		}
	}
	canon, err := doc.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(raw), bytes.TrimSpace(canon)) {
		t.Errorf("the template is not in canonical export form.\n--- want ---\n%s", canon)
	}
}

// TestRepoProfiles_CredentialNotIncluded establishes why the directory is safe for contribution:
// a profile with any field out of format — the way a credential leak
// would have — is rejected by the same parser that validates the directory.
func TestRepoProfiles_CredentialNotIncluded(t *testing.T) {
	base := `{"format":"seedgrid.tracker-profile","site":{"name":"X","host":"x.example"},"family":"unit3d","schema_version":1,` +
		`"taxonomy":{"category":[{"id":1,"name":"Filmes","kind":"movie"}],"type":[],"resolution":[]}`
	if _, err := ParseDocument([]byte(base + "}")); err != nil {
		t.Fatalf("the base document should be valid: %v", err)
	}
	for _, extra := range []string{`"api_token":"secret"`, `"announce":"https://x.example/announce/passkey"`, `"passkey":"abc"`, `"image_host_key":"k"`} {
		if _, err := ParseDocument([]byte(base + "," + extra + "}")); err == nil {
			t.Errorf("profile with %s was accepted", extra)
		}
	}
}
