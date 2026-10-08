// Package notify sends SeedGrid telemetry out of the Portal (ADR-028): Telegram,
// Discord and generic webhook.
//
// Principles that shape this package:
//
//   - **Best-effort, always.** Notifying is a side effect, not part of the transaction. A
//     down Telegram must NEVER prevent the Alert Worker from logging an offline Node.
//     Failure becomes a log; no one upstream gets an error.
//   - **Channel without credentials is off.** There is no separate "enabled" flag that could
//     diverge from the credentials being filled.
//   - **The operator chooses what arrives** (track filters, catalog ADR-023, hot-reload):
//     sending everything causes a flood and sending too little defeats the purpose.
//   - **Explicit anti-flood.** A flapping Node cannot become a machine gun on the phone; the
//     excess is discarded and COUNTED (silence without explanation is worse than the missing
//     message).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// Kind identifies the source trail — the operator filters by it.
const (
	KindAlert         = "alert"
	KindAlertResolved = "alert_resolved"
	KindEvent         = "event"
	KindActivity      = "activity"
	KindTest          = "test" // "send test" button: ignores filters, respects channels
)

// Field is an extra "Label: value" line in the notification body (e.g., Size,
// Reason, Author) — rich templates in hooks.go use it to structure the fact beyond
// the generic title. In order; empty Value skips the line (best-effort: missing data
// does not become a dangling "Label: ").
type Field struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Message is the fact to notify, already in operator language.
type Message struct {
	Kind     string  `json:"kind"`
	Type     string  `json:"type,omitempty"`     // source code/kind (NODE_OFFLINE, admission_hold…)
	Severity string  `json:"severity,omitempty"` // CRITICAL | WARNING | SUCCESS | INFO
	Title    string  `json:"title"`
	Message  string  `json:"message,omitempty"`
	Node     string  `json:"node,omitempty"`
	Fields   []Field `json:"fields,omitempty"`
	// NoDateField skips the "Date:" line at the end — used when a Field already covers the date
	// (e.g., "Completed at" in the seeding template), avoiding redundancy.
	NoDateField bool      `json:"-"`
	At          time.Time `json:"at"`
	// Lang: the language the text was built in (notify.language), so rendering
	// uses the same labels ("Node", "Date"). Empty = default.
	Lang string `json:"-"`

	// Schema/Data are the MACHINE contract of the webhook channel (#147). Fields is for
	// humans to read (Telegram/Discord render as text) and changes when someone improves
	// a sentence; automation depending on it would break with every copy adjustment.
	//
	// Schema only bumps on BREAKING changes: new field inside Data does not bump; removing
	// a field or changing the meaning of an existing one bumps. The consumer checks the number and
	// fails loud instead of processing wrong in silence.
	//
	// Both omitempty: message without structured block comes out exactly as it always has —
	// no current webhook consumer breaks.
	Schema int            `json:"schema,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

// SchemaVersion is the current version of the webhook `data` block (#147).
const SchemaVersion = 1

// severityRank orders severity for the minimum filter. Unknown = INFO (do not
// swallow what is not understood).
var severityRank = map[string]int{"INFO": 1, "WARNING": 2, "CRITICAL": 3}

func rank(s string) int {
	if r, ok := severityRank[strings.ToUpper(s)]; ok {
		return r
	}
	return 1
}

// Service decides what goes out and where.
type Service struct {
	cfg    *settings.Store
	logger *slog.Logger
	http   *http.Client

	// telegramAPI is injectable so tests don't hit the real Telegram API.
	telegramAPI string

	mu sync.Mutex
	// win holds the ceiling window per CHANNEL: a single shared ceiling let
	// a burst on one channel discard another's message.
	win map[string]*rateWindow
	// suppressed counts what is discarded per channel and becomes a metric — silence needs to be
	// observable, otherwise the operator thinks the cluster is quiet.
	suppressedTotal *prometheus.CounterVec
}

// rateWindow is the sliding window for a channel.
type rateWindow struct {
	start      time.Time
	sent       int
	suppressed int
}

// New creates the service over the tunables catalog (hot-reload: everything is read per send).
func New(cfg *settings.Store, logger *slog.Logger) *Service {
	return &Service{
		win: map[string]*rateWindow{},
		suppressedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "seedgrid_notify_suppressed_total",
			Help: "Notifications dropped by the per-minute cap, per channel and track. Alerts never hit the cap.",
		}, []string{"channel", "kind"}),
		cfg:         cfg,
		logger:      logger,
		http:        &http.Client{Timeout: 10 * time.Second},
		telegramAPI: "https://api.telegram.org",
	}
}

// Notify sends in the background and returns no error: it is the domain's usage point, and no
// flow that generated a fact can be affected by an external channel (ADR-028).
func (s *Service) Notify(m Message) {
	if !s.wantedByAnyChannel(m) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := s.deliver(ctx, m); err != nil {
			s.logger.Warn("notification not delivered", "error", err, "kind", m.Kind, "title", m.Title)
		}
	}()
}

// Send delivers synchronously and returns the error — used by the "send test" button, where
// the operator MUST see what went wrong (invalid token, wrong chat_id).
func (s *Service) Send(ctx context.Context, m Message) error { return s.deliver(ctx, m) }

// Channels addressable by the per-channel test (ADR-028).
const (
	ChannelTelegram = "telegram"
	ChannelDiscord  = "discord"
	ChannelWebhook  = "webhook"
)

// allChannels is the dispatch order. Each channel decides on its own if it wants the message.
var allChannels = []string{ChannelTelegram, ChannelDiscord, ChannelWebhook}

// Configured says if there is at least one channel with credentials.
func (s *Service) Configured() bool {
	return s.ChannelConfigured(ChannelTelegram) || s.ChannelConfigured(ChannelDiscord) ||
		s.ChannelConfigured(ChannelWebhook)
}

// ChannelConfigured says if a specific channel has credentials.
func (s *Service) ChannelConfigured(ch string) bool {
	switch ch {
	case ChannelTelegram:
		return s.telegramReady()
	case ChannelDiscord:
		return s.cfg.String("notify.discord_webhook_url") != ""
	case ChannelWebhook:
		return s.cfg.String("notify.webhook_url") != ""
	}
	return false
}

// SendToChannel delivers the message to ONE channel — used by each channel's test button,
// so the operator knows exactly which token is right. Ignores filters and rate-limit (it's
// a test). Errors if the channel is not configured or if delivery fails (redacted, no token).
func (s *Service) SendToChannel(ctx context.Context, ch string, m Message) error {
	if m.At.IsZero() {
		m.At = time.Now()
	}
	switch ch {
	case ChannelTelegram:
		if !s.telegramReady() {
			return fmt.Errorf("telegram without token and/or chat_id")
		}
		return s.sendTelegram(ctx, m)
	case ChannelDiscord:
		u := s.cfg.String("notify.discord_webhook_url")
		if u == "" {
			return fmt.Errorf("discord without webhook URL")
		}
		return s.sendDiscord(ctx, u, m)
	case ChannelWebhook:
		u := s.cfg.String("notify.webhook_url")
		if u == "" {
			return fmt.Errorf("webhook without URL")
		}
		return s.sendWebhook(ctx, u, m)
	}
	return fmt.Errorf("unknown channel: %q", ch)
}

func (s *Service) telegramReady() bool {
	return s.cfg.String("notify.telegram_bot_token") != "" && s.cfg.String("notify.telegram_chat_id") != ""
}

// chFilters is the set of filters that apply to ONE channel: its own, if the operator
// enabled them, otherwise the general ones.
type chFilters struct {
	minSeverity string
	onResolve   bool
	events      string
	activities  string
	rateLimit   int
}

// filtersFor resolves a channel's filters. The `custom` toggle decides between its own and
// the general ones — not "empty means inherit", because empty ALREADY has meaning in
// tracks (empty list = no events). A channel without `custom` behaves as before
// routing existed: nothing changes for those already using it.
func (s *Service) filtersFor(ch string) chFilters {
	p := "notify." + ch + "."
	if s.cfg.Bool(p + "custom") {
		return chFilters{
			minSeverity: s.cfg.String(p + "alerts_min_severity"),
			onResolve:   s.cfg.Bool(p + "alerts_on_resolve"),
			events:      s.cfg.String(p + "events"),
			activities:  s.cfg.String(p + "activities"),
			rateLimit:   s.cfg.Int(p + "rate_limit_per_min"),
		}
	}
	return chFilters{
		minSeverity: s.cfg.String("notify.alerts_min_severity"),
		onResolve:   s.cfg.Bool("notify.alerts_on_resolve"),
		events:      s.cfg.String("notify.events"),
		activities:  s.cfg.String("notify.activities"),
		rateLimit:   s.cfg.Int("notify.rate_limit_per_min"),
	}
}

// shouldSendTo applies the filters of ONE channel (ADR-028 + per-channel routing). The test
// bypasses them: whoever clicked wants to see the message arrive, regardless of filters.
func (s *Service) shouldSendTo(ch string, m Message) bool {
	if m.Kind == KindTest {
		return true
	}
	f := s.filtersFor(ch)
	switch m.Kind {
	case KindAlert, KindAlertResolved:
		if f.minSeverity == "OFF" {
			return false
		}
		if m.Kind == KindAlertResolved && !f.onResolve {
			return false
		}
		return rank(m.Severity) >= rank(f.minSeverity)
	case KindEvent:
		return listMatches(f.events, m.Type)
	case KindActivity:
		return listMatches(f.activities, m.Type)
	}
	return false
}

// wantedByAnyChannel avoids waking a goroutine for a message that no channel wants.
func (s *Service) wantedByAnyChannel(m Message) bool {
	for _, ch := range allChannels {
		if s.ChannelConfigured(ch) && s.shouldSendTo(ch, m) {
			return true
		}
	}
	return false
}

// listMatches parses the operator's list: empty = nothing, "*" = everything, otherwise CSV
// (case-insensitive comparison, tolerating spaces — it's a hand-typed field).
func listMatches(list, typ string) bool {
	list = strings.TrimSpace(list)
	if list == "" {
		return false
	}
	if list == "*" {
		return true
	}
	for _, item := range strings.Split(list, ",") {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(typ)) {
			return true
		}
	}
	return false
}

// allowOn applies the per-minute ceiling FOR ONE CHANNEL. Returns false when the message must
// be discarded, accounting for the suppression (log + metric).
//
// ⚠️ ALERT NEVER hits the ceiling, in any channel. Alerts exist to bother, and a
// limiter that DISCARDS might eat the very warning that mattered — which one dies is
// arbitrary (whichever arrived after the quota burst). Before this change, a burst of
// "download complete" could swallow a CRITICAL, and the discard only showed up in a
// logger.Warn inside the container log.
//
// Against an alert storm the remedy is different: DB dedup (index
// uk_alerts_open_condition) already prevents re-notifying while the alert is open, and
// noisy rules are fixed in the RULE. If protection is ever needed here, let it be
// coalescence ("N alerts"), never discard.
func (s *Service) allowOn(ch string, m Message) bool {
	switch m.Kind {
	case KindTest, KindAlert, KindAlertResolved:
		return true
	}
	limit := s.filtersFor(ch).rateLimit
	if limit <= 0 {
		return true // 0 = no ceiling
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.win[ch]
	if w == nil {
		w = &rateWindow{}
		s.win[ch] = w
	}
	now := time.Now()
	if now.Sub(w.start) >= time.Minute {
		// Closes the window: if there was a discard, SAY so — silence without explanation would make the
		// operator think the cluster is quiet.
		if w.suppressed > 0 {
			s.logger.Warn("notifications suppressed by the per-minute cap",
				"channel", ch, "suppressed", w.suppressed, "limit", limit)
		}
		w.start, w.sent, w.suppressed = now, 0, 0
	}
	if w.sent >= limit {
		w.suppressed++
		s.suppressedTotal.WithLabelValues(ch, m.Kind).Inc()
		return false
	}
	w.sent++
	return true
}

// Collectors exposes the service metrics to the /metrics registry. The CP's
// registry is private (doesn't use the default registry), so the collector must be handed over.
func (s *Service) Collectors() []prometheus.Collector {
	return []prometheus.Collector{s.suppressedTotal}
}

// deliver sends the message through all configured channels. Errors are aggregated: a broken
// channel doesn't stop the others (Discord going down can't silence Telegram).
func (s *Service) deliver(ctx context.Context, m Message) error {
	if m.At.IsZero() {
		m.At = time.Now()
	}
	var errs []string
	for _, ch := range allChannels {
		if !s.ChannelConfigured(ch) {
			continue
		}
		// Each channel applies ITS OWN filters and ITS OWN cap: this is what allows sending only the
		// IPv6 switch to the webhook that triggers automation, without the endpoint receiving
		// everything the cluster notifies.
		if !s.shouldSendTo(ch, m) || !s.allowOn(ch, m) {
			continue
		}
		if err := s.sendOne(ctx, ch, m); err != nil {
			errs = append(errs, ch+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// sendOne dispatches to an already chosen channel (filters and cap resolved by the caller).
func (s *Service) sendOne(ctx context.Context, ch string, m Message) error {
	switch ch {
	case ChannelTelegram:
		return s.sendTelegram(ctx, m)
	case ChannelDiscord:
		return s.sendDiscord(ctx, s.cfg.String("notify.discord_webhook_url"), m)
	case ChannelWebhook:
		return s.sendWebhook(ctx, s.cfg.String("notify.webhook_url"), m)
	}
	return fmt.Errorf("unknown channel: %q", ch)
}

// icon gives the immediate visual hint — whoever reads on mobile decides in 1 second whether to get up.
// Green is explicit success (Severity SUCCESS or resolved alert); without this, the absence
// of severity falls to INFO/blue — success is never the silent default.
func icon(m Message) string {
	if m.Kind == KindAlertResolved {
		return "🟢"
	}
	switch strings.ToUpper(m.Severity) {
	case "CRITICAL":
		return "🔴"
	case "WARNING":
		return "🟠"
	case "SUCCESS":
		return "🟢"
	default:
		return "🔵"
	}
}

// text builds the common body (used by Telegram and Discord): icon + title, the Fields of the
// template in order (empty Field skips the line), the Node and, by default, the date at the end —
// NoDateField skips when a Field already covers it (e.g.: "Completed at").
//
// The Node comes AFTER the purpose Fields: whoever reads on mobile sees the subject of the
// event (the file, the size) first and only then on which machine it happened. With the Node right below
// the title, "Download completed" was followed by the hostname and looked like the file name.
func text(m Message) string {
	var b strings.Builder
	b.WriteString(icon(m) + " " + m.Title)
	for _, f := range m.Fields {
		if f.Value == "" {
			continue
		}
		b.WriteString("\n" + f.Label + ": " + f.Value)
	}
	if m.Node != "" {
		b.WriteString("\n" + tr(m.Lang, "line.node") + ": " + m.Node)
	}
	if m.Message != "" {
		b.WriteString("\n" + m.Message)
	}
	if !m.NoDateField {
		b.WriteString("\n" + tr(m.Lang, "line.date") + ": " + m.At.Format(tr(m.Lang, "layout.date")))
	}
	return b.String()
}

// humanBytes formats bytes in the same convention as the Portal (portal/src/components/ui.tsx
// fmtBytes) — whoever already reads sizes there won't find the format strange in the notification.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func (s *Service) sendTelegram(ctx context.Context, m Message) error {
	// No parse_mode: the text comes from torrent name/hostname and may contain characters
	// that Telegram's Markdown rejects (400 "can't parse entities"). Plain text always
	// delivers — readability is not worth the risk of losing the alert.
	body := map[string]string{
		"chat_id": s.cfg.String("notify.telegram_chat_id"),
		"text":    text(m),
	}
	endpoint := s.telegramAPI + "/bot" + s.cfg.String("notify.telegram_bot_token") + "/sendMessage"
	return s.postJSON(ctx, endpoint, body)
}

func (s *Service) sendDiscord(ctx context.Context, webhookURL string, m Message) error {
	return s.postJSON(ctx, webhookURL, map[string]any{"content": text(m)})
}

// sendWebhook sends the raw Message: whoever receives it (n8n, Home Assistant, Slack via bridge)
// decides what to do. It is the channel that requires no new code per integration.
func (s *Service) sendWebhook(ctx context.Context, target string, m Message) error {
	return s.postJSON(ctx, target, m)
}

func (s *Service) postJSON(ctx context.Context, endpoint string, payload any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return redact(err, endpoint)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// redact strips the URL from the network error: it carries the bot token (…/bot<TOKEN>/sendMessage) and
// the webhook URL, and this error goes to the log and to the test button screen (ADR-028).
func redact(err error, endpoint string) error {
	msg := err.Error()
	if u, pErr := url.Parse(endpoint); pErr == nil {
		msg = strings.ReplaceAll(msg, endpoint, u.Scheme+"://"+u.Host+"/…")
	}
	return fmt.Errorf("%s", msg)
}
