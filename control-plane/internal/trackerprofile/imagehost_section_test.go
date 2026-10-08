package trackerprofile

import (
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
)

// ---- ParseDocument with image_host -----------------------------------------------------

// A valid Spec passes validation and enters the clean document.
func TestParseDocument_ImageHostValid(t *testing.T) {
	raw := []byte(`{
		"format": "seedgrid.tracker-profile",
		"site": {"name": "S", "host": "tracker.exemplo.org"},
		"family": "unit3d",
		"schema_version": 1,
		"taxonomy": {},
		"image_host": {
			"name": "Host do site",
			"upload_url": "https://img.tracker.exemplo.org/api/v1/sharex/upload",
			"file_field": "file",
			"auth_header": "Authorization",
			"auth_prefix": "Bearer ",
			"url_path": "url",
			"thumbnail_path": "thumbnail_url"
		}
	}`)
	doc, err := ParseDocument(raw)
	if err != nil {
		t.Fatalf("ParseDocument failed: %v", err)
	}
	if doc.ImageHost == nil {
		t.Fatal("ImageHost is nil, want populated")
	}
	if doc.ImageHost.Name != "Host do site" {
		t.Errorf("Name = %q, want %q", doc.ImageHost.Name, "Host do site")
	}
	if doc.ImageHost.UploadURL != "https://img.tracker.exemplo.org/api/v1/sharex/upload" {
		t.Errorf("UploadURL = %q", doc.ImageHost.UploadURL)
	}
}

// An invalid Spec is rejected with the field path.
func TestParseDocument_ImageHostInvalid(t *testing.T) {
	cases := []struct {
		name, body, field string
	}{
		{
			"upload_url without https",
			`{"format":"seedgrid.tracker-profile","site":{"name":"S","host":"tracker.exemplo.org"},"family":"unit3d","schema_version":1,"taxonomy":{},"image_host":{"name":"Host","upload_url":"http://img.tracker.org/upload","file_field":"file","auth_header":"Authorization","auth_prefix":"","url_path":"url"}}`,
			"image_host.upload_url",
		},
		{
			"empty file_field",
			`{"format":"seedgrid.tracker-profile","site":{"name":"S","host":"tracker.exemplo.org"},"family":"unit3d","schema_version":1,"taxonomy":{},"image_host":{"name":"Host","upload_url":"https://img.tracker.org/upload","file_field":"","auth_header":"Authorization","auth_prefix":"","url_path":"url"}}`,
			"image_host.file_field",
		},
		{
			"auth_header proibido",
			`{"format":"seedgrid.tracker-profile","site":{"name":"S","host":"tracker.exemplo.org"},"family":"unit3d","schema_version":1,"taxonomy":{},"image_host":{"name":"Host","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Host","auth_prefix":"","url_path":"url"}}`,
			"image_host.auth_header",
		},
		{
			"empty url_path",
			`{"format":"seedgrid.tracker-profile","site":{"name":"S","host":"tracker.exemplo.org"},"family":"unit3d","schema_version":1,"taxonomy":{},"image_host":{"name":"Host","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Authorization","auth_prefix":"","url_path":""}}`,
			"image_host.url_path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDocument([]byte(tc.body))
			if err == nil {
				t.Fatal("expected error, passed")
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error %q does not mention field %q", err.Error(), tc.field)
			}
		})
	}
}

// ---- Section-unit merge on import -------------------------------------------------

// No local section: applies (status new).
func TestPlanImageHost_NoLocalSection(t *testing.T) {
	cur := Profile{}
	doc := Document{
		Site:    Site{Name: "S", Host: "tracker.exemplo.org"},
		Profile: Profile{ImageHost: &imagehost.Spec{Name: "Host", UploadURL: "https://img.tracker.org/upload", FileField: "file", AuthHeader: "Authorization", URLPath: "url"}},
	}
	plan, err := PlanImport(cur, doc, Site{}, nil)
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}
	if plan.ImageHost == nil {
		t.Fatal("plan.ImageHost is nil")
	}
	if plan.ImageHost.Status != StatusNew {
		t.Errorf("Status = %q, want %q", plan.ImageHost.Status, StatusNew)
	}
}

// Same section: nothing (status same).
func TestPlanImageHost_SameSection(t *testing.T) {
	spec := imagehost.Spec{Name: "Host", UploadURL: "https://img.tracker.org/upload", FileField: "file", AuthHeader: "Authorization", URLPath: "url"}
	cur := Profile{ImageHost: &spec}
	doc := Document{
		Site:    Site{Name: "S", Host: "tracker.exemplo.org"},
		Profile: Profile{ImageHost: &spec},
	}
	plan, err := PlanImport(cur, doc, Site{}, nil)
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}
	if plan.ImageHost == nil {
		t.Fatal("plan.ImageHost is nil")
	}
	if plan.ImageHost.Status != StatusSame {
		t.Errorf("Status = %q, want %q", plan.ImageHost.Status, StatusSame)
	}
}

// Different section without overwrite: protected (status protected).
func TestPlanImageHost_DifferentSectionProtected(t *testing.T) {
	cur := Profile{ImageHost: &imagehost.Spec{Name: "Meu Host", UploadURL: "https://meu.tracker.org/upload", FileField: "file", AuthHeader: "Authorization", URLPath: "url"}}
	file := imagehost.Spec{Name: "Host", UploadURL: "https://img.tracker.org/upload", FileField: "file", AuthHeader: "Authorization", URLPath: "url"}
	doc := Document{
		Site:    Site{Name: "S", Host: "tracker.exemplo.org"},
		Profile: Profile{ImageHost: &file},
	}
	plan, err := PlanImport(cur, doc, Site{}, nil)
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}
	if plan.ImageHost == nil {
		t.Fatal("plan.ImageHost is nil")
	}
	if plan.ImageHost.Status != StatusProtected {
		t.Errorf("Status = %q, want %q", plan.ImageHost.Status, StatusProtected)
	}
	if plan.ImageHost.Current == nil {
		t.Error("Current is nil, want populated")
	}
}

// Different section with overwrite: replaced (status replaced).
func TestPlanImageHost_DifferentSectionWithOverwrite(t *testing.T) {
	cur := Profile{ImageHost: &imagehost.Spec{Name: "Meu Host", UploadURL: "https://meu.tracker.org/upload", FileField: "file", AuthHeader: "Authorization", URLPath: "url"}}
	file := imagehost.Spec{Name: "Host", UploadURL: "https://img.tracker.org/upload", FileField: "file", AuthHeader: "Authorization", URLPath: "url"}
	doc := Document{
		Site:    Site{Name: "S", Host: "tracker.exemplo.org"},
		Profile: Profile{ImageHost: &file},
	}
	overwrite := map[OverwriteKey]bool{ImageHostKey: true}
	plan, err := PlanImport(cur, doc, Site{}, overwrite)
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}
	if plan.ImageHost == nil {
		t.Fatal("plan.ImageHost is nil")
	}
	if plan.ImageHost.Status != StatusReplaced {
		t.Errorf("Status = %q, want %q", plan.ImageHost.Status, StatusReplaced)
	}
	if !plan.ImageHost.OverwritesProtected {
		t.Error("OverwritesProtected = false, want true")
	}
}

// ---- Export ---------------------------------------------------------------------------

// The section format in the file is part of the exchange contract.
func TestExport_WithImageHostSection(t *testing.T) {
	spec := imagehost.Spec{
		Name:          "Host do site",
		UploadURL:     "https://img.tracker.exemplo.org/api/v1/sharex/upload",
		FileField:     "file",
		AuthHeader:    "Authorization",
		AuthPrefix:    "Bearer ",
		URLPath:       "url",
		ThumbnailPath: "thumbnail_url",
	}
	p := Profile{
		Family:        FamilyUNIT3D,
		SchemaVersion: SchemaVersion,
		Taxonomy:      Taxonomy{},
		ImageHost:     &spec,
	}
	doc := NewDocument("Tracker", "tracker.exemplo.org", p)
	raw, err := doc.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	const want = `  "image_host": {
    "name": "Host do site",
    "upload_url": "https://img.tracker.exemplo.org/api/v1/sharex/upload",
    "file_field": "file",
    "auth_header": "Authorization",
    "auth_prefix": "Bearer ",
    "url_path": "url",
    "thumbnail_path": "thumbnail_url"
  }
}
`
	if !strings.HasSuffix(string(raw), want) {
		t.Errorf("file does not end with formatted section (which comes after taxonomy):\n%s", raw)
	}
}
