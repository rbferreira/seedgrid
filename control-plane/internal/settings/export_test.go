package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const exportPath = "../../../shared/settings.json"

// exportedSpec is what the Portal needs to show the setting in the reader's language: the key and
// the Portuguese text that the Control Plane sends today (the fallback), plus the choice values.
type exportedSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Choices     []Choice `json:"choices,omitempty"`
}

type exported struct {
	// Categories: the panel groups (each name comes from the Portal catalog).
	Categories []Category `json:"categories"`
	// Specs: the manually written settings.
	Specs []exportedSpec `json:"specs"`
	// Channels and ChannelSpecs: those generated per channel (notify.<channel>.<suffix>). The label comes
	// without the channel name, which the Portal puts in the {channel} of the sentence.
	Channels     []string       `json:"channels"`
	ChannelSpecs []exportedSpec `json:"channel_specs"`
}

func exportCatalog() exported {
	var out exported
	seen := map[Category]bool{}
	for _, s := range catalog {
		if !seen[s.Category] {
			seen[s.Category] = true
			out.Categories = append(out.Categories, s.Category)
		}
	}
	for _, s := range baseCatalog {
		out.Specs = append(out.Specs, exportedSpec{Key: s.Key, Label: s.Label, Description: s.Description, Choices: s.Choices})
	}
	for _, ch := range notifyChannels {
		out.Channels = append(out.Channels, ch.key)
	}
	first := notifyChannels[0]
	for _, s := range channelOverrideSpecs() {
		if !strings.HasPrefix(s.Key, "notify."+first.key+".") {
			continue
		}
		out.ChannelSpecs = append(out.ChannelSpecs, exportedSpec{
			Key:         strings.TrimPrefix(s.Key, "notify."+first.key+"."),
			Label:       strings.TrimPrefix(s.Label, first.label+" — "),
			Description: s.Description,
			Choices:     s.Choices,
		})
	}
	return out
}

// TestCatalogExported: shared/settings.json is the settings catalog, for the Portal to display
// the label and description in the reader's language (ADR-041 §4) and reject settings without translation.
// To regenerate after changing the catalog: UPDATE_SETTINGS=1 go test ./internal/settings
func TestCatalogExported(t *testing.T) {
	want, err := json.MarshalIndent(exportCatalog(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if os.Getenv("UPDATE_SETTINGS") == "1" {
		if err := os.WriteFile(exportPath, want, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read %s: %v (generate with UPDATE_SETTINGS=1 go test ./internal/settings)", exportPath, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Errorf("%s is outdated relative to catalog: run UPDATE_SETTINGS=1 go test ./internal/settings", exportPath)
	}
}

// Every channel generates the same settings, with the label "<Channel> — <suffix>": this is what allows
// exporting a single template, with {channel}.
func TestSettingsByChannelFollowModel(t *testing.T) {
	specs := channelOverrideSpecs()
	per := len(specs) / len(notifyChannels)
	for i, ch := range notifyChannels {
		for j := 0; j < per; j++ {
			s := specs[i*per+j]
			if !strings.HasPrefix(s.Key, "notify."+ch.key+".") || !strings.HasPrefix(s.Label, ch.label+" — ") {
				t.Errorf("%s (%q) does not follow the pattern notify.<channel>.<suffix> / \"<Channel> — …\"", s.Key, s.Label)
			}
		}
	}
}
