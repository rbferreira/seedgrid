package notify

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// Adapters from domain tracks to external telemetry (ADR-028).
//
// They stay HERE and not in the source packages to keep the dependency one-way:
// `alert`/`events`/`activity` declare the interface they need and don't know `notify`;
// `notify` satisfies it structurally. main is what connects the two — changing the output
// channel (or removing it) doesn't touch the domain.

// AlertHook connects the Alert Service (ADR-008) to telemetry. Satisfies alert.Notifier.
type AlertHook struct{ S *Service }

func (h AlertHook) AlertOpened(code, severity, nodeID, message string, payload map[string]any) {
	l := h.S.lang()
	m := Message{
		Kind:     KindAlert,
		Type:     code,
		Severity: severity,
		Title:    titleFor(l, "alert", code, message),
		Node:     hostnameOf(payload, nodeID),
		At:       time.Now(),
		Lang:     l,
	}
	applyAlertTemplate(&m, code, payload)
	h.S.Notify(m)
}

func (h AlertHook) AlertResolved(code, severity, nodeID, message string, payload map[string]any) {
	l := h.S.lang()
	h.S.Notify(Message{
		Kind:     KindAlertResolved,
		Type:     code,
		Severity: severity,
		Title:    tr(l, "resolved") + ": " + titleFor(l, "alert", code, message),
		Node:     hostnameOf(payload, nodeID),
		At:       time.Now(),
		Lang:     l,
	})
}

// EventHook connects the Event emitter (ADR-006) to telemetry. Satisfies events.Notifier.
type EventHook struct{ S *Service }

func (h EventHook) EventEmitted(evType, severity, nodeID, message string, payload map[string]any) {
	l := h.S.lang()
	m := Message{
		Kind:     KindEvent,
		Type:     evType,
		Severity: severity,
		Title:    titleFor(l, "event", evType, message),
		Node:     hostnameOf(payload, nodeID),
		At:       time.Now(),
		Lang:     l,
	}
	applyEventTemplate(&m, evType, payload)
	h.S.Notify(m)
}

// ActivityHook connects the orchestration Feed (ADR-022) to telemetry. Satisfies activity.Notifier.
type ActivityHook struct{ S *Service }

func (h ActivityHook) ActivityRecorded(kind, message, torrentName, nodeID string, detail map[string]any) {
	l := h.S.lang()
	m := Message{
		Kind:     KindActivity,
		Type:     kind,
		Severity: "INFO", // orchestration is informative by default; specific templates override it (e.g., seeding = success)
		Title:    titleFor(l, "activity", kind, message),
		Message:  torrentName,
		Node:     hostnameOf(detail, nodeID),
		At:       time.Now(),
		Lang:     l,
	}
	applyActivityTemplate(&m, kind, torrentName, detail)
	h.S.Notify(m)
}

// titleOr avoids notifications with an empty title: without a message, the code/kind at least says
// what happened.
func titleOr(message, fallback string) string {
	if message != "" {
		return message
	}
	return fallback
}

// hostnameOf extracts the hostname that the caller already resolved and recorded in the payload/detail
// (same pattern as the Alert Worker — see alert/worker.go). Without it, it falls back to the Node id —
// harder to read, but better than omitting which machine it is.
func hostnameOf(payload map[string]any, nodeID string) string {
	if payload != nil {
		if h, ok := payload["hostname"].(string); ok && h != "" {
			return h
		}
	}
	return nodeID
}

// --- Rich Templates (ADR-028) ---
//
// They replace the generic body (Title + Message) with structured fields (Node + Fields
// + Data) for EVERY notifiable Alert/Event/Activity — not just the three original
// examples (seeding, aborted migration, removed torrent). Each case uses only what the
// payload/detail already carries (see the call sites in torrent.go/migration.go/api.go/
// alert/worker.go); nothing here triggers a new query. Code without a matching case
// keeps the generic body already assembled by the Hook (Title/Message as they came).

const (
	// activityKindSeeding, activityKindAdmissionHold, activityKindScheduled mirror
	// activity.Kind* — hardcoded so notify doesn't import the domain package (same
	// structural independence as the rest of the file).
	activityKindSeeding       = "seeding"
	activityKindAdmissionHold = "admission_hold"
	activityKindScheduled     = "scheduled"
	activityKindNodeBlocked   = "node_blocked"

	// eventCode* mirror the codes emitted by torrent.go/migration.go/node.go/api.go.
	eventCodeTorrentCreated    = "TORRENT_CREATED"
	eventCodeTorrentRemoved    = "TORRENT_REMOVED"
	eventCodeTorrentPaused     = "TORRENT_PAUSED"
	eventCodeTorrentResumed    = "TORRENT_RESUMED"
	eventCodeTorrentRecheck    = "TORRENT_RECHECK"
	eventCodeTorrentReseeded   = "TORRENT_RESEEDED"
	eventCodeMigrationStarted  = "TORRENT_MIGRATION_STARTED"
	eventCodeMigrated          = "TORRENT_MIGRATED"
	eventCodeMigrationFailed   = "TORRENT_MIGRATION_FAILED"
	eventCodePublishedExternal = "TORRENT_PUBLISHED_EXTERNAL"
	eventCodeNodeRegistered    = "NODE_REGISTERED"
	eventCodeNodeDeleted       = "NODE_DELETED"
	eventCodeEngineRestart     = "ENGINE_RESTART"
	eventCodeAgentUpdate       = "AGENT_UPDATE"
	eventCodeStorageRecovery   = "STORAGE_RECOVERY"

	// alertCode* mirror the codes exposed by alert/worker.go and the
	// storage-recovery endpoint (api.go).
	alertCodeNodeStorage           = "NODE_STORAGE"
	alertCodeNodeDiskFull          = "NODE_DISK_FULL"
	alertCodeDiskIOSaturated       = "DISK_IO_SATURATED"
	alertCodeStorageRecoveryFailed = "STORAGE_RECOVERY_FAILED"
	alertCodeNodeIPv6Changed       = "NODE_IPV6_CHANGED"
	alertCodeNodeInboundSilent     = "NODE_INBOUND_SILENT"
	alertCodeNodeFirewalled        = "NODE_FIREWALLED"
)

// ipv6Prefix reduces an IPv6 address to its /64 — the first four hextets.
//
// This is what the firewall rule NEEDS: the 05-06/09/2026 incident happened because the
// rules used a literal address instead of the network object, so they survived the prefix
// rotation pointing to a dead address. SeedGrid calculates this and delivers it ready
// in the `data` block; letting the consumer derive it with string manipulation would be planting the
// same bug on the other side. Returns "" when it cannot safely reduce it.
func ipv6Prefix(addr string) string {
	if addr == "" || strings.Contains(addr, "::") {
		// Compressed form: safely expanding requires a full parse, and a wrong
		// prefix is worse than no prefix — the consumer has the full address.
		if ip := net.ParseIP(addr); ip != nil && ip.To4() == nil {
			if m := ip.Mask(net.CIDRMask(64, 128)); m != nil {
				return m.String() + "/64"
			}
		}
		return ""
	}
	parts := strings.Split(addr, ":")
	if len(parts) < 4 {
		return ""
	}
	return strings.Join(parts[:4], ":") + "::/64"
}

// applyActivityTemplate enriches the Activity with structured Fields. "seeding" gets the
// full template (Name/Size/Added on/Completed on, success/green); the other
// kinds get at least "File" + whatever the detail carries (Reason/Score/Candidates).
func applyActivityTemplate(m *Message, kind, torrentName string, detail map[string]any) {
	if kind == activityKindSeeding {
		m.Severity = "SUCCESS"
		m.Title = tr(m.Lang, "title.activity.seeding")
		m.Message = ""
		m.NoDateField = true // "Completed on" below is already the date
		fields := []Field{{Label: tr(m.Lang, "label.name"), Value: torrentName}}
		if size, ok := int64From(detail["size_bytes"]); ok {
			fields = append(fields, Field{Label: tr(m.Lang, "label.size"), Value: humanBytes(size)})
		}
		if added, ok := timeFrom(detail["added_at"]); ok {
			fields = append(fields, Field{Label: tr(m.Lang, "label.added_at"), Value: added.Format(tr(m.Lang, "layout.date"))})
		}
		fields = append(fields, Field{Label: tr(m.Lang, "label.completed_at"), Value: m.At.Format(tr(m.Lang, "layout.date"))})
		m.Fields = fields
		return
	}

	m.Message = ""
	fields := []Field{{Label: tr(m.Lang, "label.file"), Value: torrentName}}
	switch kind {
	case activityKindAdmissionHold:
		if reason := reasonText(m.Lang, detail); reason != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.reason"), Value: reason})
		}
	case activityKindScheduled:
		if reason := reasonText(m.Lang, detail); reason != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.reason"), Value: reason})
		}
		if score, ok := detail["score"]; ok {
			fields = append(fields, Field{Label: tr(m.Lang, "label.score"), Value: fmt.Sprint(score)})
		}
		if cand, ok := detail["candidates"]; ok {
			fields = append(fields, Field{Label: tr(m.Lang, "label.candidates"), Value: fmt.Sprint(cand)})
		}
	case activityKindNodeBlocked:
		// The Node left out is the actionable information: that's where the operator will
		// check the disk. Without this, the message would only say that the torrent didn't enter (#180).
		m.Severity = "WARNING"
		m.Title = tr(m.Lang, "title.activity.node_blocked")
		if nodes := stringsFrom(detail["nodes"]); len(nodes) > 0 {
			fields = append(fields, Field{Label: tr(m.Lang, "label.node_out"), Value: strings.Join(nodes, ", ")})
		}
		if reason := reasonText(m.Lang, detail); reason != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.reason"), Value: reason})
		}
	}
	m.Fields = fields
}

// reasonText: the reason for an activity in the notification language, by reason_code; records
// without a code (prior to #33) fall back to the recorded text.
func reasonText(lang string, detail map[string]any) string {
	if code := stringFrom(detail["reason_code"]); code != "" {
		if t := tr(lang, "reason."+code); t != "" {
			return t
		}
	}
	return stringFrom(detail["reason"])
}

// applyEventTemplate covers every notifiable Event with a short Title + Fields — not just the
// two original examples (aborted migration, removed torrent).
func applyEventTemplate(m *Message, code string, payload map[string]any) {
	switch code {
	case eventCodeTorrentCreated:
		m.Title = tr(m.Lang, "title.event.TORRENT_CREATED")
		if _, published := payload["private"]; published {
			m.Title = tr(m.Lang, "title.event.TORRENT_CREATED.created")
		}
		m.Fields = []Field{{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)}}
	case eventCodeTorrentPaused:
		m.Title = tr(m.Lang, "title.event.TORRENT_PAUSED")
		m.Fields = []Field{{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)}}
	case eventCodeTorrentResumed:
		m.Title = tr(m.Lang, "title.event.TORRENT_RESUMED")
		m.Fields = []Field{{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)}}
	case eventCodeTorrentRecheck:
		m.Title = tr(m.Lang, "title.event.TORRENT_RECHECK")
		m.Fields = []Field{{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)}}
	case eventCodeTorrentReseeded:
		m.Title = tr(m.Lang, "title.event.TORRENT_RESEEDED")
		m.Fields = []Field{
			{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)},
			{Label: tr(m.Lang, "label.old_hash"), Value: stringFrom(payload["old_hash"])},
			{Label: tr(m.Lang, "label.new_hash"), Value: stringFrom(payload["new_hash"])},
		}
	case eventCodeMigrationStarted:
		m.Title = tr(m.Lang, "title.event.TORRENT_MIGRATION_STARTED")
		m.Fields = []Field{
			{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)},
			{Label: tr(m.Lang, "label.destination"), Value: stringFrom(payload["dest_hostname"])},
		}
	case eventCodeMigrated:
		m.Severity = "SUCCESS" // migration completed successfully — same criteria as seeding
		m.Title = tr(m.Lang, "title.event.TORRENT_MIGRATED")
		m.Fields = []Field{
			{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)},
			{Label: tr(m.Lang, "label.source"), Value: stringFrom(payload["source_hostname"])},
		}
	case eventCodeMigrationFailed:
		m.Title = tr(m.Lang, "title.event.TORRENT_MIGRATION_FAILED")
		m.Fields = []Field{
			{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)},
			{Label: tr(m.Lang, "label.reason"), Value: stringFrom(payload["reason"])},
		}
	case eventCodeTorrentRemoved:
		m.Title = tr(m.Lang, "title.event.TORRENT_REMOVED")
		actor := stringFrom(payload["actor"])
		if actor == "" {
			actor = "SeedGrid" // no actor = automatic reconciliation, not operator action
		}
		m.Fields = []Field{
			{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)},
			{Label: tr(m.Lang, "label.author"), Value: actor},
		}
	case eventCodePublishedExternal:
		m.Title = tr(m.Lang, "title.event.TORRENT_PUBLISHED_EXTERNAL")
		m.Fields = []Field{
			{Label: tr(m.Lang, "label.tracker"), Value: stringFrom(payload["tracker"])},
			{Label: tr(m.Lang, "label.file"), Value: fileLabel(payload)},
		}
	case eventCodeNodeRegistered:
		m.Severity = "SUCCESS" // onboarding completed successfully — same criteria as seeding
		m.Title = tr(m.Lang, "title.event.NODE_REGISTERED")
		if v := stringFrom(payload["agent_version"]); v != "" {
			m.Fields = []Field{{Label: tr(m.Lang, "label.agent_version"), Value: v}}
		}
	case eventCodeNodeDeleted:
		m.Title = tr(m.Lang, "title.event.NODE_DELETED")
		var fields []Field
		if n, ok := payload["torrents_removed"]; ok {
			fields = append(fields, Field{Label: tr(m.Lang, "label.torrents_removed"), Value: fmt.Sprint(n)})
		}
		if forced, ok := payload["force"].(bool); ok && forced {
			fields = append(fields, Field{Label: tr(m.Lang, "label.forced"), Value: tr(m.Lang, "yes")})
		}
		m.Fields = fields
	case eventCodeEngineRestart:
		m.Title = tr(m.Lang, "title.event.ENGINE_RESTART")
	case eventCodeAgentUpdate:
		m.Title = tr(m.Lang, "title.event.AGENT_UPDATE")
		var fields []Field
		if from := stringFrom(payload["from"]); from != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.from"), Value: from})
		}
		if to := stringFrom(payload["to"]); to != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.to"), Value: to})
		}
		m.Fields = fields
	case eventCodeStorageRecovery:
		var fields []Field
		if step := stringFrom(payload["step"]); step != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.step"), Value: step})
		}
		if att, ok := payload["attempt"]; ok {
			fields = append(fields, Field{Label: tr(m.Lang, "label.attempt"), Value: fmt.Sprint(att)})
		}
		m.Fields = fields
	}
}

// applyAlertTemplate covers Alerts with extra data beyond the Node — the other conditions
// (e.g., NODE_OFFLINE) already say everything in the Title and stick to just Node + Data.
func applyAlertTemplate(m *Message, code string, payload map[string]any) {
	switch code {
	case alertCodeNodeStorage:
		if detail := stringFrom(payload["detail"]); detail != "" {
			m.Fields = []Field{{Label: tr(m.Lang, "label.detail"), Value: detail}}
		}
	case alertCodeNodeDiskFull:
		projFree, okP := int64From(payload["projected_free_bytes"])
		minFree, okM := int64From(payload["min_free_bytes"])
		if okP && okM {
			m.Fields = []Field{
				{Label: tr(m.Lang, "label.missing"), Value: humanBytes(minFree - projFree)},
				{Label: tr(m.Lang, "label.min_floor"), Value: humanBytes(minFree)},
			}
		}
	case alertCodeDiskIOSaturated:
		if util, ok := payload["util_pct"].(float64); ok {
			m.Fields = []Field{{Label: tr(m.Lang, "label.io_util"), Value: fmt.Sprintf("%.0f%%", util)}}
		}
	case alertCodeNodeIPv6Changed:
		// Only alert with a `data` block for now (#147): it exists to TRIGGER
		// automation (n8n rewrites the firewall rules), not just to warn. Fields
		// remains what the human reads; Data is what the machine consumes.
		prev := stringFrom(payload["previous_ipv6"])
		cur := stringFrom(payload["current_ipv6"])
		m.Fields = []Field{
			{Label: tr(m.Lang, "label.previous"), Value: prev},
			{Label: tr(m.Lang, "label.current"), Value: cur},
			{Label: tr(m.Lang, "label.bt_port"), Value: fmt.Sprint(payload["bt_port"])},
		}
		m.Schema = SchemaVersion
		m.Data = map[string]any{
			"event": "node_ipv6_changed",
			"node": map[string]any{
				"id":       stringFrom(payload["node_id"]),
				"hostname": stringFrom(payload["hostname"]),
				"bt_port":  payload["bt_port"],
			},
			"ipv6": map[string]any{
				"previous":        prev,
				"current":         cur,
				"prefix_previous": ipv6Prefix(prev),
				"prefix_current":  ipv6Prefix(cur),
			},
		}
	case alertCodeNodeInboundSilent:
		// The alert text is intentionally neutral (#203): it is recorded once and the Node
		// address changes when the provider rotates the prefix. This notification, on the
		// other hand, is an INSTANT — the payload address is the one from when the alert
		// opened, it is correct here and saves a trip to the Node to find out what to unblock.
		var fields []Field
		if ep := btEndpoint(payload); ep != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.node_address"), Value: ep})
		}
		if s := stringFrom(payload["last_incoming_at"]); s != "" {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				fields = append(fields, Field{Label: tr(m.Lang, "label.last_inbound"), Value: t.In(m.At.Location()).Format(tr(m.Lang, "layout.date"))})
			}
		}
		m.Fields = fields
	case alertCodeNodeFirewalled:
		// Same class of problem as NODE_INBOUND_SILENT (#223): the text is recorded once
		// and the Node address changes when the provider rotates the prefix. This notification
		// is an INSTANT — the payload address is the one from when the alert opened, it is
		// correct here and saves a trip to the Node to find out what to unblock.
		var fields []Field
		if ep := btEndpoint(payload); ep != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.node_address"), Value: ep})
		}
		if s := stringFrom(payload["connectable_since"]); s != "" {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				fields = append(fields, Field{Label: tr(m.Lang, "label.firewalled_since"), Value: t.In(m.At.Location()).Format(tr(m.Lang, "layout.date"))})
			}
		}
		m.Fields = fields
	case alertCodeStorageRecoveryFailed:
		var fields []Field
		if step := stringFrom(payload["step"]); step != "" {
			fields = append(fields, Field{Label: tr(m.Lang, "label.step"), Value: step})
		}
		if att, ok := payload["attempt"]; ok {
			fields = append(fields, Field{Label: tr(m.Lang, "label.attempt"), Value: fmt.Sprint(att)})
		}
		m.Fields = fields
	}
}

// fileLabel prefers the torrent name; falls back to the hash when there is no name yet — same
// criteria as the label() used by the domain when assembling the original message.
func fileLabel(payload map[string]any) string {
	if name := stringFrom(payload["name"]); name != "" {
		return name
	}
	return stringFrom(payload["hash"])
}

func stringFrom(v any) string {
	s, _ := v.(string)
	return s
}

// stringsFrom reads a text list from the detail. Accepts []string (the normal case, where the
// detail comes straight from the recorder) and []any (in case it ever goes through JSON, which loses the
// concrete type) — this way the template doesn't go blank due to a serialization round-trip.
func stringsFrom(v any) []string {
	switch xs := v.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func int64From(v any) (int64, bool) {
	n, ok := v.(int64)
	return n, ok
}

// btEndpoint builds "[address]:port" from the payload of a reachability alert.
// Without an address it returns "" — never a "[]:6881" that would look like an instruction. Without a port, just the
// address. The port arrives as int (straight from the worker) or float64 (after a round-trip
// through JSON), so it accepts both.
func btEndpoint(payload map[string]any) string {
	addr := stringFrom(payload["public_ipv6"])
	if addr == "" {
		return ""
	}
	var port int64
	switch n := payload["bt_port"].(type) {
	case int:
		port = int64(n)
	case int64:
		port = n
	case float64:
		port = int64(n)
	}
	if port <= 0 {
		return addr
	}
	return fmt.Sprintf("[%s]:%d", addr, port)
}

func timeFrom(v any) (time.Time, bool) {
	t, ok := v.(time.Time)
	return t, ok
}
