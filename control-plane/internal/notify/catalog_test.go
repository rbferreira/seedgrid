package notify

import (
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/messages"
)

// Both languages have the same keys: a key in only one of them would silently fall back to the default.
func TestCatalogSameKeys(t *testing.T) {
	for lang, texts := range catalog {
		for k := range catalog[defaultLang] {
			if _, ok := texts[k]; !ok {
				t.Errorf("%s: missing key %q", lang, k)
			}
		}
		for k := range texts {
			if _, ok := catalog[defaultLang][k]; !ok {
				t.Errorf("%s: key %q does not exist in the default language", lang, k)
			}
		}
	}
}

// Every alert, event and activity code in the registry (ADR-041 §3) has a title in both
// languages — a new code without a title fails here, as it fails in the Portal catalog.
func TestCatalogCoversRecord(t *testing.T) {
	for _, m := range messages.Registry {
		if m.Kind == messages.Error || m.Kind == messages.Audit {
			continue // errors belong to the API and audit actions to the trail: neither becomes a notification
		}
		key := "title." + string(m.Kind) + "." + m.Code
		for lang, texts := range catalog {
			if texts[key] == "" {
				t.Errorf("%s: no title for %s", lang, key)
			}
		}
	}
}

// The notification goes out in the language set in notify.language, and the default is English.
func TestNotificationInClusterLanguage(t *testing.T) {
	ev := func(lang string) Message {
		over := map[string]string{}
		if lang != "" {
			over["notify.language"] = lang
		}
		s := svc(t, over)
		l := s.lang()
		m := Message{Kind: KindEvent, Type: "TORRENT_PAUSED", Title: titleFor(l, "event", "TORRENT_PAUSED", "Torrent pausado: x"), Lang: l}
		applyEventTemplate(&m, "TORRENT_PAUSED", map[string]any{"name": "Debian"})
		return m
	}
	if m := ev(""); m.Title != "Torrent paused" || m.Fields[0].Label != "File" {
		t.Errorf("default: %q / %q, want English", m.Title, m.Fields[0].Label)
	}
	if m := ev("pt-BR"); m.Title != "Torrent pausado" || m.Fields[0].Label != "Arquivo" {
		t.Errorf("pt-BR: %q / %q", m.Title, m.Fields[0].Label)
	}
	if got := text(ev("pt-BR")); !strings.Contains(got, "Data: ") {
		t.Errorf("pt-BR: the date line should be \"Data:\": %q", got)
	}
	if got := text(ev("en")); !strings.Contains(got, "Date: ") {
		t.Errorf("en: the date line should be \"Date:\": %q", got)
	}
}

// The activity reason comes from reason_code, in the language; an old record without a code shows the recorded text.
func TestReasonByCode(t *testing.T) {
	if got := reasonText("en", map[string]any{"reason": "taxa baixa", "reason_code": "slow"}); got != "slow, gave up its slot" {
		t.Errorf("with code: %q", got)
	}
	if got := reasonText("en", map[string]any{"reason": "taxa baixa"}); got != "taxa baixa" {
		t.Errorf("without code: %q", got)
	}
}
