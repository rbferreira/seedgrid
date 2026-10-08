package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// capture logs what each channel received (the test never touches the real network).
type capture struct {
	mu     sync.Mutex
	paths  []string
	bodies []string
}

func (c *capture) add(path, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paths = append(c.paths, path)
	c.bodies = append(c.bodies, body)
}
func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}
func (c *capture) last() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bodies) == 0 {
		return ""
	}
	return c.bodies[len(c.bodies)-1]
}

func server(t *testing.T, c *capture, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.add(r.URL.Path, string(b))
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func svc(t *testing.T, over map[string]string) *Service {
	t.Helper()
	s := New(settings.NewForTest(over), slog.New(slog.NewTextHandler(io.Discard, nil)))
	return s
}

func msg(kind, typ, sev string) Message {
	return Message{Lang: "pt-BR", Kind: kind, Type: typ, Severity: sev, Title: "Node caiu", Node: "seedgrid-node-03", At: time.Unix(1780000000, 0)}
}

// Telegram: the token goes in the PATH (/bot<TOKEN>/sendMessage) and the chat_id in the body.
func TestTelegram(t *testing.T) {
	c := &capture{}
	srv := server(t, c, 200)
	s := svc(t, map[string]string{
		"notify.telegram_bot_token": "123:ABC",
		"notify.telegram_chat_id":   "-100999",
	})
	s.telegramAPI = srv.URL

	if err := s.Send(context.Background(), msg(KindAlert, "NODE_OFFLINE", "CRITICAL")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if c.count() != 1 {
		t.Fatalf("sends = %d, want 1", c.count())
	}
	if got := c.paths[0]; got != "/bot123:ABC/sendMessage" {
		t.Errorf("path = %q, want /bot123:ABC/sendMessage", got)
	}
	var body map[string]string
	_ = json.Unmarshal([]byte(c.last()), &body)
	if body["chat_id"] != "-100999" {
		t.Errorf("chat_id = %q", body["chat_id"])
	}
	if !strings.Contains(body["text"], "Node caiu") || !strings.Contains(body["text"], "seedgrid-node-03") {
		t.Errorf("text without title/node: %q", body["text"])
	}
}

// Channel without credentials is turned off (ADR-028) — there is no separate flag.
func TestChannelWithoutCredentialDoesNotSend(t *testing.T) {
	c := &capture{}
	srv := server(t, c, 200)
	// token without chat_id: incomplete ⇒ does not send
	s := svc(t, map[string]string{"notify.telegram_bot_token": "123:ABC"})
	s.telegramAPI = srv.URL

	if s.Configured() {
		t.Error("Configured() should be false: Telegram without chat_id does not send")
	}
	if err := s.Send(context.Background(), msg(KindAlert, "X", "CRITICAL")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if c.count() != 0 {
		t.Errorf("sent %d times with incomplete channel; want 0", c.count())
	}
}

func TestDiscordEWebhook(t *testing.T) {
	dc, wh := &capture{}, &capture{}
	dsrv, wsrv := server(t, dc, 204), server(t, wh, 200)
	s := svc(t, map[string]string{
		"notify.discord_webhook_url": dsrv.URL + "/hook",
		"notify.webhook_url":         wsrv.URL + "/generic",
	})
	m := msg(KindAlert, "NODE_OFFLINE", "CRITICAL")
	m.Message = "sem heartbeat há 90s"
	if err := s.Send(context.Background(), m); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var d map[string]any
	_ = json.Unmarshal([]byte(dc.last()), &d)
	if !strings.Contains(d["content"].(string), "Node caiu") {
		t.Errorf("discord content: %v", d["content"])
	}
	// The generic webhook receives the RAW Message — it's the contract that spares code per
	// integration; breaking field names breaks the n8n/HA of those who already integrated.
	var g Message
	if err := json.Unmarshal([]byte(wh.last()), &g); err != nil {
		t.Fatalf("webhook did not receive Message JSON: %v", err)
	}
	if g.Kind != KindAlert || g.Type != "NODE_OFFLINE" || g.Severity != "CRITICAL" ||
		g.Title != "Node caiu" || g.Node != "seedgrid-node-03" || g.Message != "sem heartbeat há 90s" {
		t.Errorf("generic webhook payload incomplete: %+v", g)
	}
}

// A broken channel cannot silence the others.
func TestChannelBrokenDoesNotBlockOthers(t *testing.T) {
	ok, bad := &capture{}, &capture{}
	oksrv, badsrv := server(t, ok, 200), server(t, bad, 500)
	s := svc(t, map[string]string{
		"notify.discord_webhook_url": badsrv.URL, // failure
		"notify.webhook_url":         oksrv.URL,  // must receive it anyway
	})
	err := s.Send(context.Background(), msg(KindAlert, "X", "CRITICAL"))
	if err == nil {
		t.Error("Send should report the broken channel error")
	}
	if ok.count() != 1 {
		t.Error("healthy channel should have received even with the other failing")
	}
}

func TestSeverityFilter(t *testing.T) {
	cases := []struct {
		min, sev string
		want     bool
	}{
		{"WARNING", "CRITICAL", true},
		{"WARNING", "WARNING", true},
		{"WARNING", "INFO", false},
		{"CRITICAL", "WARNING", false},
		{"CRITICAL", "CRITICAL", true},
		{"INFO", "INFO", true},
		{"OFF", "CRITICAL", false}, // OFF turns off the track without messing with the channels
	}
	for _, c := range cases {
		s := svc(t, map[string]string{"notify.alerts_min_severity": c.min})
		if got := s.shouldSendTo(ChannelTelegram, msg(KindAlert, "X", c.sev)); got != c.want {
			t.Errorf("min=%s sev=%s → %v, want %v", c.min, c.sev, got, c.want)
		}
	}
}

func TestResolvedFilter(t *testing.T) {
	on := svc(t, map[string]string{"notify.alerts_on_resolve": "true", "notify.alerts_min_severity": "WARNING"})
	if !on.shouldSendTo(ChannelTelegram, msg(KindAlertResolved, "X", "CRITICAL")) {
		t.Error("with alerts_on_resolve=true, resolution should notify")
	}
	off := svc(t, map[string]string{"notify.alerts_on_resolve": "false", "notify.alerts_min_severity": "WARNING"})
	if off.shouldSendTo(ChannelTelegram, msg(KindAlertResolved, "X", "CRITICAL")) {
		t.Error("with alerts_on_resolve=false, resolution should NOT notify")
	}
	if !off.shouldSendTo(ChannelTelegram, msg(KindAlert, "X", "CRITICAL")) {
		t.Error("disabling the resolution notification must not affect the opening alert")
	}
}

// High-volume tracks are born disabled (empty default) — the default must be useful
// without being unbearable.
func TestEventsAndActivitiesDefaultDisabled(t *testing.T) {
	s := svc(t, nil)
	if s.shouldSendTo(ChannelTelegram, msg(KindEvent, "TORRENT_REMOVED", "INFO")) {
		t.Error("notify.events empty (default) should not notify anything")
	}
	if s.shouldSendTo(ChannelTelegram, msg(KindActivity, "admission_hold", "INFO")) {
		t.Error("notify.activities empty (default) should not notify anything")
	}
}

func TestTypeList(t *testing.T) {
	s := svc(t, map[string]string{"notify.activities": " admission_hold , seeding "})
	if !s.shouldSendTo(ChannelTelegram, msg(KindActivity, "admission_hold", "INFO")) {
		t.Error("listed kind should pass (tolerating spaces — it is a manually typed field)")
	}
	if !s.shouldSendTo(ChannelTelegram, msg(KindActivity, "SEEDING", "INFO")) {
		t.Error("comparison should be case-insensitive")
	}
	if s.shouldSendTo(ChannelTelegram, msg(KindActivity, "downloading", "INFO")) {
		t.Error("kind outside the list should not pass")
	}
	star := svc(t, map[string]string{"notify.events": "*"})
	if !star.shouldSendTo(ChannelTelegram, msg(KindEvent, "QUALQUER_COISA", "INFO")) {
		t.Error("* should allow all")
	}
}

// ⚠️ DELIBERATE INVERSION. This test asserted that 10 alerts became 3 due to the cap.
// Alert is now EXEMPT from the cap, in any channel: alerts exist to bother, and a
// rate limiter that DISCARDS might eat exactly the warning that mattered — which one dies is
// arbitrary (whoever arrives after the quota bursts). Against a storm, the remedy is the
// dedup (which already exists in the DB) and fixing the noisy rule, not dropping on delivery.
func TestRateLimitDoesNotApplyToAlert(t *testing.T) {
	c := &capture{}
	srv := server(t, c, 200)
	s := svc(t, map[string]string{
		"notify.webhook_url":         srv.URL,
		"notify.rate_limit_per_min":  "3",
		"notify.alerts_min_severity": "INFO",
	})
	for i := 0; i < 10; i++ {
		_ = s.Send(context.Background(), msg(KindAlert, "X", "CRITICAL"))
	}
	if c.count() != 10 {
		t.Errorf("delivered %d of 10 alerts; alert cannot be discarded by cap", c.count())
	}
}

// Event and Activity remain capped — the cap exists precisely for them.
func TestRateLimitAppliesToEvent(t *testing.T) {
	c := &capture{}
	srv := server(t, c, 200)
	s := svc(t, map[string]string{
		"notify.webhook_url":        srv.URL,
		"notify.rate_limit_per_min": "3",
		"notify.events":             "*",
	})
	for i := 0; i < 10; i++ {
		_ = s.Send(context.Background(), msg(KindEvent, "TORRENT_CREATED", "INFO"))
	}
	if c.count() != 3 {
		t.Errorf("sent %d; the 3/min ceiling should have discarded the excess", c.count())
	}
	s.mu.Lock()
	sup := s.win[ChannelWebhook].suppressed
	s.mu.Unlock()
	if sup != 7 {
		t.Errorf("suppressed = %d, want 7 (discarding must be counted, not silent)", sup)
	}
}

// The case that motivated it all: a burst of events cannot prevent the delivery of a
// CRITICAL alert. Before, the window was single and shared across all Kinds.
func TestEventBurstDoesNotSwallowAlert(t *testing.T) {
	c := &capture{}
	srv := server(t, c, 200)
	s := svc(t, map[string]string{
		"notify.webhook_url":         srv.URL,
		"notify.rate_limit_per_min":  "2",
		"notify.events":              "*",
		"notify.alerts_min_severity": "INFO",
	})
	for i := 0; i < 20; i++ {
		_ = s.Send(context.Background(), msg(KindEvent, "TORRENT_CREATED", "INFO"))
	}
	before := c.count()
	_ = s.Send(context.Background(), msg(KindAlert, "NODE_OFFLINE", "CRITICAL"))
	if c.count() != before+1 {
		t.Error("the CRITICAL alert was discarded after a burst of events — exactly the bug")
	}
}

// Routing by channel: with its own filters, the webhook receives ONLY what was requested,
// while Telegram (without override) follows the general filters.
func TestRoutingByChannel(t *testing.T) {
	tg, wh := &capture{}, &capture{}
	tsrv, wsrv := server(t, tg, 200), server(t, wh, 200)
	s := svc(t, map[string]string{
		"notify.telegram_bot_token": "T",
		"notify.telegram_chat_id":   "1",
		"notify.webhook_url":        wsrv.URL,
		// general: alert from WARNING, no events
		"notify.alerts_min_severity": "WARNING",
		"notify.events":              "",
		// webhook: only the IPv6 change event, no alerts
		"notify.webhook.custom":              "true",
		"notify.webhook.alerts_min_severity": "OFF",
		"notify.webhook.events":              "NODE_IPV6_CHANGED",
	})
	s.telegramAPI = tsrv.URL

	_ = s.deliver(context.Background(), msg(KindAlert, "NODE_OFFLINE", "CRITICAL"))
	_ = s.deliver(context.Background(), msg(KindEvent, "NODE_IPV6_CHANGED", "INFO"))

	if tg.count() != 1 {
		t.Errorf("telegram received %d, want 1 (only the alert; event is unchecked overall)", tg.count())
	}
	if wh.count() != 1 {
		t.Errorf("webhook received %d, want 1 (only the IPv6 event; alert is OFF on it)", wh.count())
	}
	if wh.count() > 0 && !strings.Contains(wh.last(), "NODE_IPV6_CHANGED") {
		t.Errorf("webhook received the wrong message: %s", wh.last())
	}
}

// Test BY CHANNEL (ADR-028): sends only to the requested channel, and refuses unconfigured channels —
// so the operator knows exactly which token is wrong.
func TestSendToChannel(t *testing.T) {
	dc, wh := &capture{}, &capture{}
	dsrv, wsrv := server(t, dc, 204), server(t, wh, 200)
	s := svc(t, map[string]string{
		"notify.discord_webhook_url": dsrv.URL,
		"notify.webhook_url":         wsrv.URL,
		// telegram purposely NOT configured
	})

	if !s.ChannelConfigured(ChannelDiscord) || s.ChannelConfigured(ChannelTelegram) {
		t.Fatal("ChannelConfigured does not reflect channel state")
	}
	// Configured channel: delivers only to it.
	if err := s.SendToChannel(context.Background(), ChannelDiscord, msg(KindTest, "", "INFO")); err != nil {
		t.Fatalf("SendToChannel discord: %v", err)
	}
	if dc.count() != 1 || wh.count() != 0 {
		t.Errorf("discord=%d webhook=%d; per-channel test must touch ONLY discord", dc.count(), wh.count())
	}
	// Unconfigured channel: clear error, without trying to deliver.
	if err := s.SendToChannel(context.Background(), ChannelTelegram, msg(KindTest, "", "INFO")); err == nil {
		t.Error("SendToChannel on an unconfigured channel should fail")
	}
}

// The operator's test must arrive even with filters closed: they clicked to see it.
func TestKindTestIgnoresFilters(t *testing.T) {
	c := &capture{}
	srv := server(t, c, 200)
	s := svc(t, map[string]string{
		"notify.webhook_url":         srv.URL,
		"notify.alerts_min_severity": "OFF",
		"notify.rate_limit_per_min":  "1",
	})
	for i := 0; i < 3; i++ {
		if err := s.Send(context.Background(), Message{Lang: "pt-BR", Kind: KindTest, Title: "teste"}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	if c.count() != 3 {
		t.Errorf("test sends = %d, want 3 (the test ignores filter and ceiling)", c.count())
	}
}

// The channel error goes to the log and the test button screen — it cannot take the token.
func TestErrorDoesNotLeakToken(t *testing.T) {
	s := svc(t, map[string]string{
		"notify.telegram_bot_token": "123:SUPERSECRETO",
		"notify.telegram_chat_id":   "1",
	})
	s.telegramAPI = "http://127.0.0.1:1" // connection refused → network error with the URL inside
	err := s.Send(context.Background(), msg(KindAlert, "X", "CRITICAL"))
	if err == nil {
		t.Fatal("expected network error")
	}
	if strings.Contains(err.Error(), "SUPERSECRETO") {
		t.Errorf("the bot token LEAKED in the error message: %v", err)
	}
}

// Green is reserved for explicit success — absence of severity falls to INFO/blue, never
// to green by omission (that was the bug: "completed" came out blue).
func TestIconGreenOnlyForExplicitSuccess(t *testing.T) {
	if got := icon(Message{Lang: "pt-BR", Severity: "SUCCESS"}); got != "🟢" {
		t.Errorf("icon SUCCESS = %q, want 🟢", got)
	}
	if got := icon(Message{Lang: "pt-BR", Kind: KindAlertResolved, Severity: "INFO"}); got != "🟢" {
		t.Errorf("icon AlertResolved = %q, want 🟢", got)
	}
	if got := icon(Message{Lang: "pt-BR", Severity: "INFO"}); got != "🔵" {
		t.Errorf("icon INFO/default = %q, want 🔵 (success cannot be the silent default)", got)
	}
	if got := icon(Message{Lang: "pt-BR", Severity: "WARNING"}); got != "🟠" {
		t.Errorf("icon WARNING = %q, want 🟠", got)
	}
	if got := icon(Message{Lang: "pt-BR", Severity: "CRITICAL"}); got != "🔴" {
		t.Errorf("icon CRITICAL = %q, want 🔴", got)
	}
}

// text() structures Fields (in order) + Node + Date at the end — the rich body of ADR-028.
// Order matters: the Node comes AFTER the Fields so the hostname isn't read as the
// file name right below the title.
func TestTextRendersNodeFieldsEData(t *testing.T) {
	m := Message{Lang: "pt-BR",
		Severity: "WARNING", Title: "Migração de torrent abortada",
		Node:   "seedgrid-node-01",
		Fields: []Field{{Label: "Arquivo", Value: "filme.mkv"}, {Label: "Motivo", Value: "disk full"}},
		At:     time.Unix(1780000000, 0),
	}
	got := text(m)
	for _, want := range []string{"🟠 Migração de torrent abortada", "Node: seedgrid-node-01", "Arquivo: filme.mkv", "Motivo: disk full", "Data: "} {
		if !strings.Contains(got, want) {
			t.Errorf("text without %q: %q", want, got)
		}
	}
	if strings.Index(got, "Node: ") < strings.Index(got, "Arquivo: ") {
		t.Errorf("Node should come after Fields, not right below the title: %q", got)
	}
	if strings.Index(got, "Data: ") < strings.Index(got, "Node: ") {
		t.Errorf("Date should be the last line: %q", got)
	}
}

// Field with empty Value doesn't turn into a dangling "Label: ".
func TestTextSkipEmptyField(t *testing.T) {
	m := Message{Lang: "pt-BR", Title: "x", Fields: []Field{{Label: "Motivo", Value: ""}}}
	if strings.Contains(text(m), "Motivo") {
		t.Error("Field with empty Value should not appear in the text")
	}
}

// NoDateField avoids redundant "Date:" when a Field already covers the date (e.g., "Completed on").
func TestTextSkipsDataWithNoDateField(t *testing.T) {
	m := Message{Lang: "pt-BR", Title: "x", NoDateField: true, At: time.Unix(1780000000, 0)}
	if strings.Contains(text(m), "Data:") {
		t.Error("NoDateField=true should not render the line Date:")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{500, "500 B"},
		{2048, "2 KB"},
		{5 * 1 << 20, "5 MB"},
		{int64(1.5 * (1 << 30)), "1.50 GB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// seeding template: Name/Size/Added on/Completed on, success (green) and without
// redundant "Date:" — exactly what the operator requested ("completed" must come out green).
func TestApplyActivityTemplateSeeding(t *testing.T) {
	m := Message{Lang: "pt-BR", Kind: KindActivity, Severity: "INFO", At: time.Unix(1780000000, 0)}
	detail := map[string]any{"size_bytes": int64(1610612736), "added_at": time.Unix(1779990000, 0)}
	applyActivityTemplate(&m, activityKindSeeding, "filme.mkv", detail)

	if m.Severity != "SUCCESS" {
		t.Errorf("severity = %q, want SUCCESS", m.Severity)
	}
	if !m.NoDateField {
		t.Error("seeding should not repeat Date: (Completed on already covers it)")
	}
	f := fieldMap(m.Fields)
	if f["Nome"] != "filme.mkv" {
		t.Errorf("Name = %q", f["Nome"])
	}
	if f["Tamanho"] != "1.50 GB" {
		t.Errorf("Size = %q, want 1.50 GB", f["Tamanho"])
	}
	if f["Adicionado em"] == "" || f["Concluído em"] == "" {
		t.Error("Added on / Completed on should not be empty")
	}
}

// Kinds without a dedicated rich template (e.g., downloading, admission_resume) still get the
// structured "File" — they just don't have extra Fields nor turn into success/green.
func TestApplyActivityTemplateOtherKindsGetFile(t *testing.T) {
	m := Message{Lang: "pt-BR", Severity: "INFO", Title: "Começou a baixar"}
	applyActivityTemplate(&m, "downloading", "filme.mkv", nil) // mirrors activity.KindDownloading
	if m.Severity != "INFO" {
		t.Errorf("severity = %q, want INFO (only seeding is a success)", m.Severity)
	}
	if got := fieldMap(m.Fields)["Arquivo"]; got != "filme.mkv" {
		t.Errorf("File = %q, want filme.mkv", got)
	}
}

// admission_hold structures the Reason (admission controller's reason) as a Field.
func TestApplyActivityTemplateAdmissionHold(t *testing.T) {
	m := Message{Lang: "pt-BR", Severity: "INFO"}
	applyActivityTemplate(&m, activityKindAdmissionHold, "filme.mkv", map[string]any{"reason": "slow rate"})
	f := fieldMap(m.Fields)
	if f["Arquivo"] != "filme.mkv" || f["Motivo"] != "slow rate" {
		t.Errorf("fields = %+v", f)
	}
}

// scheduled structures Reason/Score/Candidates from the Scheduler's detail.
func TestApplyActivityTemplateScheduled(t *testing.T) {
	m := Message{Lang: "pt-BR", Severity: "INFO"}
	applyActivityTemplate(&m, activityKindScheduled, "filme.mkv",
		map[string]any{"reason": "melhor score", "score": 87, "candidates": 3})
	f := fieldMap(m.Fields)
	if f["Motivo"] != "melhor score" || f["Pontuação"] != "87" || f["Candidatos"] != "3" {
		t.Errorf("fields = %+v", f)
	}
}

func TestApplyEventTemplateMigrationAborted(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyEventTemplate(&m, eventCodeMigrationFailed, map[string]any{"name": "filme.mkv", "reason": "disco cheio no destino"})
	if m.Title != "Migração de torrent abortada" {
		t.Errorf("title = %q", m.Title)
	}
	f := fieldMap(m.Fields)
	if f["Arquivo"] != "filme.mkv" || f["Motivo"] != "disco cheio no destino" {
		t.Errorf("fields = %+v", f)
	}
}

// Author: real username when it came from manual removal; "SeedGrid" when the automatic
// reconciliation has no actor (torrent disappeared from inventory on its own).
func TestApplyEventTemplateTorrentRemoved(t *testing.T) {
	manual := Message{Lang: "pt-BR"}
	applyEventTemplate(&manual, eventCodeTorrentRemoved, map[string]any{"name": "filme.mkv", "actor": "rafael"})
	if got := fieldMap(manual.Fields)["Autor"]; got != "rafael" {
		t.Errorf("author = %q, want rafael", got)
	}

	auto := Message{Lang: "pt-BR"}
	applyEventTemplate(&auto, eventCodeTorrentRemoved, map[string]any{"hash": "abc123"})
	f := fieldMap(auto.Fields)
	if f["Autor"] != "SeedGrid" {
		t.Errorf("author without actor = %q, want SeedGrid (automatic reconciliation)", f["Autor"])
	}
	if f["Arquivo"] != "abc123" {
		t.Errorf("file should fall back to hash when name is missing: %q", f["Arquivo"])
	}
}

// Migration completed and Node registered are success/green — same criteria as seeding.
func TestApplyEventTemplateMigratedAndSuccess(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyEventTemplate(&m, eventCodeMigrated, map[string]any{"name": "filme.mkv", "source_hostname": "seedgrid-node-01"})
	if m.Severity != "SUCCESS" {
		t.Errorf("severity = %q, want SUCCESS", m.Severity)
	}
	if m.Title != "Torrent migrado com sucesso" {
		t.Errorf("title = %q", m.Title)
	}
	f := fieldMap(m.Fields)
	if f["Arquivo"] != "filme.mkv" || f["Origem"] != "seedgrid-node-01" {
		t.Errorf("fields = %+v", f)
	}
}

func TestApplyEventTemplateNodeRegisteredAndSuccess(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyEventTemplate(&m, eventCodeNodeRegistered, map[string]any{"agent_version": "v1.2.0"})
	if m.Severity != "SUCCESS" {
		t.Errorf("severity = %q, want SUCCESS", m.Severity)
	}
	if got := fieldMap(m.Fields)["Versão do Agent"]; got != "v1.2.0" {
		t.Errorf("Agent version = %q", got)
	}
}

func TestApplyEventTemplateNodeDeleted(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyEventTemplate(&m, eventCodeNodeDeleted, map[string]any{"torrents_removed": 4, "force": true})
	f := fieldMap(m.Fields)
	if f["Torrents removidos"] != "4" || f["Forçado"] != "Sim" {
		t.Errorf("fields = %+v", f)
	}
}

func TestApplyAlertTemplateDiskFull(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyAlertTemplate(&m, alertCodeNodeDiskFull, map[string]any{
		"projected_free_bytes": int64(1 << 30),     // 1 GB free projected
		"min_free_bytes":       int64(5 * 1 << 30), // floor of 5 GB
	})
	f := fieldMap(m.Fields)
	if f["Faltam"] != "4.00 GB" {
		t.Errorf("Remaining = %q, want 4.00 GB", f["Faltam"])
	}
	if f["Piso mínimo"] != "5.00 GB" {
		t.Errorf("Minimum floor = %q, want 5.00 GB", f["Piso mínimo"])
	}
}

func TestApplyAlertTemplateDiskSaturated(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyAlertTemplate(&m, alertCodeDiskIOSaturated, map[string]any{"util_pct": 97.4})
	if got := fieldMap(m.Fields)["Utilização de I/O"]; got != "97%" {
		t.Errorf("I/O utilization = %q, want 97%%", got)
	}
}

func fieldMap(fs []Field) map[string]string {
	m := make(map[string]string, len(fs))
	for _, f := range fs {
		m[f.Label] = f.Value
	}
	return m
}
