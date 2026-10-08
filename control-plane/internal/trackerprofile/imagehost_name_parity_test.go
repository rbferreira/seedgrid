package trackerprofile

import (
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
)

func TestImageHostNameMatchesCleanName(t *testing.T) {
	cases := []struct {
		name string
	}{
		{name: "name\twith tab"},
		{name: "name\nwith break"},
		{name: "name\u202Ehostile"},
		{name: "\u3164"},
		{name: strings.Repeat("a", 65)},
		{name: "valid name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, profileErr := CleanName(tc.name)
			spec := imagehost.Spec{
				Name:       tc.name,
				UploadURL:  "https://images.example.org/upload",
				FileField:  "file",
				AuthHeader: "Authorization",
				URLPath:    "url",
			}
			imageHostErr := spec.Validate()
			if (profileErr == nil) != (imageHostErr == nil) {
				t.Errorf("CleanName accepted = %t, Spec.Validate accepted = %t (error: %v)", profileErr == nil, imageHostErr == nil, imageHostErr)
			}
		})
	}
}
