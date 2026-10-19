// Package messages is the text contract between the Control Plane and its consumers (ADR-041 §3):
// every alert, event and activity code that the Control Plane logs, and the payload data
// that its phrase can use.
//
// The Control Plane continues logging `message` — it is the fallback for API consumers without
// a catalog, and what the Portal shows for a code it doesn't know yet. The actual phrase
// is built by the consumer, in the consumer's language, from the code and the payload; therefore
// no data can exist ONLY in `message`.
//
// The registry is exported to shared/messages.json (TestExportedRecord), and the Portal
// checks its catalog against this file. On the Control Plane side, TestAllCodeEmitted
// reads the source code and fails on emitted code without an entry here, entry without an emitter, and
// emission point whose payload doesn't bring what the registry promises.
package messages

// Kind is the track where the code appears.
type Kind string

const (
	Alert    Kind = "alert"
	Event    Kind = "event"
	Activity Kind = "activity"
	// Audit: the Audit action (#57). Payload is action_params; the action derived from the route
	// (route_*) has no data.
	Audit Kind = "audit"
	// Error: the API's error.code (ADR-041 §4). Payload is error.params; the consumer builds the
	// phrase and uses error.message as fallback and detail ({detail}).
	Error Kind = "error"
)

// Message describes a code.
type Message struct {
	Kind Kind   `json:"kind"`
	Code string `json:"code"`
	// Payload: the keys that EVERY point that emits this code logs. This is what the phrase can
	// use; optional keys (logged only sometimes) are not included here.
	Payload []string `json:"payload"`
	// Variant: payload key whose value chooses the phrase, when the same code describes
	// different situations (TORRENT_REMOVED by the operator or by disappearing from the inventory).
	// Variants: its possible values, as text (a boolean becomes "true"/"false").
	Variant  string   `json:"variant,omitempty"`
	Variants []string `json:"variants,omitempty"`
	// Generic: code that today covers different situations, each with its own phrase in
	// error.message (invalid_request, not_found…). The consumer shows the message until each point
	// gets its own code (#34, slice 34d). Only for the error track.
	Generic bool `json:"generic,omitempty"`
}

// Registry is the contract. Order: track, then code.
var Registry = []Message{
	// Control Plane Alerts (alert.Worker and storage recovery).
	{Kind: Alert, Code: "DISK_IO_SATURATED", Payload: []string{"hostname", "util_pct"}},
	{Kind: Alert, Code: "NODE_DISK_FULL", Payload: []string{"hostname", "projected_free_bytes", "min_free_bytes"}},
	{Kind: Alert, Code: "NODE_FIREWALLED", Payload: []string{"hostname", "public_ipv6", "bt_port", "connectable_since"}},
	{Kind: Alert, Code: "NODE_INBOUND_SILENT", Payload: []string{"hostname", "public_ipv6", "bt_port", "last_incoming_at",
		"demand_torrents", "demand_leechers", "demand_seeds"}},
	{Kind: Alert, Code: "NODE_IPV6_CHANGED", Payload: []string{"hostname", "previous_ipv6", "current_ipv6", "bt_port"}},
	{Kind: Alert, Code: "NODE_OFFLINE", Payload: []string{"hostname"}},
	{Kind: Alert, Code: "NODE_STORAGE", Payload: []string{"hostname", "detail"}},
	{Kind: Alert, Code: "STORAGE_RECOVERY_FAILED", Payload: []string{"hostname", "step", "attempt"}},
	// Alertmanager alerts: the code is the rule name in deploy/observability/rules.yml.
	{Kind: Alert, Code: "DiskSpaceLow", Payload: []string{"instance"}},
	{Kind: Alert, Code: "HighMemory", Payload: []string{"instance"}},
	{Kind: Alert, Code: "NodeExporterDown", Payload: []string{"instance"}},

	// Events.
	{Kind: Event, Code: "AGENT_UPDATE", Payload: []string{"hostname", "from", "to"}},
	{Kind: Event, Code: "ENGINE_RESTART", Payload: []string{"hostname"}},
	{Kind: Event, Code: "NODE_CONFIG_SEEDED", Payload: []string{"hostname", "overrides"}},
	{Kind: Event, Code: "NODE_DELETED", Payload: []string{"hostname", "force", "torrents_removed"}},
	{Kind: Event, Code: "NODE_REGISTERED", Payload: []string{"hostname", "agent_version", "name_dropped"},
		Variant: "name_dropped", Variants: []string{"false", "true"}},
	{Kind: Event, Code: "STORAGE_RECOVERY", Payload: []string{"hostname", "step", "attempt"},
		Variant: "step", Variants: []string{"start", "reattach", "reboot", "verify", "success", "abort"}},
	{Kind: Event, Code: "TORRENT_CREATED", Payload: []string{"hash", "name", "hostname", "origin"},
		Variant: "origin", Variants: []string{"created", "scheduled"}},
	{Kind: Event, Code: "TORRENT_MIGRATED", Payload: []string{"hash", "name", "hostname", "source_hostname"}},
	{Kind: Event, Code: "TORRENT_MIGRATION_FAILED", Payload: []string{"hash", "name", "hostname", "cancelled"},
		Variant: "cancelled", Variants: []string{"false", "true"}},
	{Kind: Event, Code: "TORRENT_MIGRATION_STARTED", Payload: []string{"hash", "name", "hostname", "dest_hostname"}},
	{Kind: Event, Code: "TORRENT_PAUSED", Payload: []string{"hash", "name", "hostname"}},
	{Kind: Event, Code: "TORRENT_PUBLISHED_EXTERNAL", Payload: []string{"tracker", "name"}},
	{Kind: Event, Code: "TORRENT_RECHECK", Payload: []string{"hash", "name", "hostname"}},
	{Kind: Event, Code: "TORRENT_REMOVED", Payload: []string{"hash", "name", "hostname", "reason"},
		Variant: "reason", Variants: []string{"operator", "inventory_reconcile"}},
	{Kind: Event, Code: "TORRENT_RESEEDED", Payload: []string{"name", "hostname", "old_hash", "new_hash"}},
	{Kind: Event, Code: "TORRENT_RESUMED", Payload: []string{"hash", "name", "hostname"}},

	// Activities (the Feed). The torrent name comes in the entry itself (torrent_name), not in the
	// payload; the phrase can always use it.
	{Kind: Activity, Code: "admission_hold", Payload: []string{"reason_code"},
		Variant: "reason_code", Variants: []string{"cap", "slow", "dead", "manual_pause_adopted"}},
	{Kind: Activity, Code: "admission_resume", Payload: []string{}},
	{Kind: Activity, Code: "downloading", Payload: []string{"hostname"}},
	{Kind: Activity, Code: "node_blocked", Payload: []string{"nodes", "reason_code"}},
	{Kind: Activity, Code: "scheduled", Payload: []string{"reason_code", "score", "candidates"},
		Variant: "reason_code", Variants: []string{"best_health", "best_health_with_space"}},
	{Kind: Activity, Code: "screenshot_host_failed", Payload: []string{"host", "reason", "reason_code"},
		Variant: "reason_code", Variants: []string{"key_mismatch", "redirect", "timeout", "connect", "no_url", "invalid_url", "http_status", "other"}},
	{Kind: Activity, Code: "seeding", Payload: []string{"hostname", "size_bytes"}},

	// API errors. Generic ones continue with error.message until they get their own codes.
	{Kind: Error, Code: "agent_artifacts_url_missing", Payload: []string{}},
	{Kind: Error, Code: "agent_update_unavailable", Payload: []string{}},
	{Kind: Error, Code: "already_current", Payload: []string{}},
	{Kind: Error, Code: "already_migrating", Payload: []string{}},
	{Kind: Error, Code: "browse_failed", Payload: []string{}},
	{Kind: Error, Code: "command_not_found", Payload: []string{}},
	{Kind: Error, Code: "db_unavailable", Payload: []string{}},
	{Kind: Error, Code: "dest_no_space", Payload: []string{}},
	{Kind: Error, Code: "dest_not_eligible", Payload: []string{}},
	{Kind: Error, Code: "discover_failed", Payload: []string{}},
	{Kind: Error, Code: "display_name_taken", Payload: []string{}},
	{Kind: Error, Code: "download_failed", Payload: []string{}},
	{Kind: Error, Code: "duplicate_name", Payload: []string{}},
	{Kind: Error, Code: "fetch_failed", Payload: []string{}},
	{Kind: Error, Code: "forbidden", Payload: []string{"permission"}},
	{Kind: Error, Code: "gate_locked", Payload: []string{}},
	{Kind: Error, Code: "igdb_disabled", Payload: []string{}},
	{Kind: Error, Code: "igdb_failed", Payload: []string{}},
	{Kind: Error, Code: "internal_error", Payload: []string{}},
	{Kind: Error, Code: "invalid_announce", Payload: []string{}},
	{Kind: Error, Code: "invalid_api_url", Payload: []string{}},
	{Kind: Error, Code: "invalid_body", Payload: []string{}},
	{Kind: Error, Code: "invalid_display_name", Payload: []string{}},
	{Kind: Error, Code: "invalid_input", Payload: []string{}},
	{Kind: Error, Code: "invalid_mode", Payload: []string{}},
	{Kind: Error, Code: "invalid_profile", Payload: []string{}},
	{Kind: Error, Code: "invalid_request", Payload: []string{}, Generic: true},
	{Kind: Error, Code: "invalid_ticket", Payload: []string{}},
	{Kind: Error, Code: "invalid_value", Payload: []string{}},
	{Kind: Error, Code: "last_admin", Payload: []string{}},
	{Kind: Error, Code: "manifest_error", Payload: []string{}},
	{Kind: Error, Code: "metrics_unavailable", Payload: []string{}},
	{Kind: Error, Code: "migration_already_cancelled", Payload: []string{}},
	{Kind: Error, Code: "migration_finished", Payload: []string{}},
	{Kind: Error, Code: "migration_in_flight", Payload: []string{}},
	{Kind: Error, Code: "migration_not_found", Payload: []string{}},
	{Kind: Error, Code: "no_binary", Payload: []string{}},
	{Kind: Error, Code: "no_eligible_node", Payload: []string{"nodes"}},
	{Kind: Error, Code: "no_hash", Payload: []string{}},
	{Kind: Error, Code: "no_node_with_space", Payload: []string{"need_bytes", "min_free_bytes", "best_free_bytes"}},
	{Kind: Error, Code: "no_torrent_file", Payload: []string{}},
	{Kind: Error, Code: "node_config_unavailable", Payload: []string{}},
	{Kind: Error, Code: "node_has_torrents", Payload: []string{}},
	{Kind: Error, Code: "node_not_eligible", Payload: []string{}},
	{Kind: Error, Code: "node_not_found", Payload: []string{}},
	{Kind: Error, Code: "not_per_node", Payload: []string{}},
	{Kind: Error, Code: "notify_channel_not_configured", Payload: []string{}},
	{Kind: Error, Code: "notify_no_channel", Payload: []string{}},
	{Kind: Error, Code: "notify_unavailable", Payload: []string{}},
	{Kind: Error, Code: "payload_too_large", Payload: []string{}},
	{Kind: Error, Code: "preview_stale", Payload: []string{}},
	{Kind: Error, Code: "profile_without_image_host", Payload: []string{}},
	{Kind: Error, Code: "prometheus_error", Payload: []string{}},
	{Kind: Error, Code: "publish_failed", Payload: []string{}},
	{Kind: Error, Code: "reset_failed", Payload: []string{}},
	{Kind: Error, Code: "same_node", Payload: []string{}},
	{Kind: Error, Code: "screenshots_failed", Payload: []string{}},
	{Kind: Error, Code: "send_failed", Payload: []string{}},
	{Kind: Error, Code: "setting_not_found", Payload: []string{}},
	{Kind: Error, Code: "source_no_endpoint", Payload: []string{}},
	{Kind: Error, Code: "torrent_file_missing", Payload: []string{}},
	{Kind: Error, Code: "torrent_not_found", Payload: []string{}},
	{Kind: Error, Code: "torrent_not_on_node", Payload: []string{}},
	{Kind: Error, Code: "tracker_not_found", Payload: []string{}},
	{Kind: Error, Code: "tracker_timeout", Payload: []string{"seconds"}},
	{Kind: Error, Code: "tracker_upload_failed", Payload: []string{}},
	{Kind: Error, Code: "tracker_without_api", Payload: []string{}},
	{Kind: Error, Code: "unauthorized", Payload: []string{}},
	{Kind: Error, Code: "unknown_arch", Payload: []string{}},
	{Kind: Error, Code: "unknown_metric", Payload: []string{}},
	{Kind: Error, Code: "unsupported_family", Payload: []string{}},
	{Kind: Error, Code: "user_not_found", Payload: []string{}},
	{Kind: Error, Code: "weak_password", Payload: []string{}},
	{Kind: Error, Code: "wrong_password", Payload: []string{}},

	// Audit actions. The route_* ones are derived from the route (no data), when the handler hasn't
	// fixed its own.
	{Kind: Audit, Code: "agent_update_requested", Payload: []string{"node", "from", "to"}},
	{Kind: Audit, Code: "cluster_mode_changed", Payload: []string{"mode"}, Variant: "mode", Variants: []string{"full_auto", "flex"}},
	{Kind: Audit, Code: "enroll_token_created", Payload: []string{"name"}},
	{Kind: Audit, Code: "login", Payload: []string{}},
	{Kind: Audit, Code: "login_failed", Payload: []string{}},
	{Kind: Audit, Code: "migration_cancelled", Payload: []string{"name", "source", "dest"}},
	{Kind: Audit, Code: "node_drained", Payload: []string{"source", "count", "dest"}},
	{Kind: Audit, Code: "node_mode_changed", Payload: []string{"mode"}, Variant: "mode", Variants: []string{"sg", "manual"}},
	{Kind: Audit, Code: "node_override_cleared", Payload: []string{"key", "label", "node", "had_before", "before", "effective"}, Variant: "had_before", Variants: []string{"false", "true"}},
	{Kind: Audit, Code: "node_override_set", Payload: []string{"key", "label", "node", "had_before", "value", "before", "effective"}, Variant: "had_before", Variants: []string{"false", "true"}},
	{Kind: Audit, Code: "node_renamed", Payload: []string{"change", "hostname", "before", "after"}, Variant: "change", Variants: []string{"none", "removed", "set", "renamed"}},
	{Kind: Audit, Code: "notify_test", Payload: []string{"channel"}},
	{Kind: Audit, Code: "notify_test_all", Payload: []string{}},
	{Kind: Audit, Code: "route_cluster_mode", Payload: []string{}},
	{Kind: Audit, Code: "route_description_edit", Payload: []string{}},
	{Kind: Audit, Code: "route_description_reset", Payload: []string{}},
	{Kind: Audit, Code: "route_download_ticket", Payload: []string{}},
	{Kind: Audit, Code: "route_engine_restart", Payload: []string{}},
	{Kind: Audit, Code: "route_enroll_token", Payload: []string{}},
	{Kind: Audit, Code: "route_image_host_edit", Payload: []string{}},
	{Kind: Audit, Code: "route_image_host_key_remove", Payload: []string{}},
	{Kind: Audit, Code: "route_image_host_key_set", Payload: []string{}},
	{Kind: Audit, Code: "route_image_host_remove", Payload: []string{}},
	{Kind: Audit, Code: "route_migration_cancel", Payload: []string{}},
	{Kind: Audit, Code: "route_node_command", Payload: []string{}},
	{Kind: Audit, Code: "route_node_drain", Payload: []string{}},
	{Kind: Audit, Code: "route_node_mode", Payload: []string{}},
	{Kind: Audit, Code: "route_node_remove", Payload: []string{}},
	{Kind: Audit, Code: "route_node_rename", Payload: []string{}},
	{Kind: Audit, Code: "route_password_change", Payload: []string{}},
	{Kind: Audit, Code: "route_profile_edit", Payload: []string{}},
	{Kind: Audit, Code: "route_profile_import", Payload: []string{}},
	{Kind: Audit, Code: "route_publish_tracker", Payload: []string{}},
	{Kind: Audit, Code: "route_setting_change", Payload: []string{}},
	{Kind: Audit, Code: "route_setting_reset", Payload: []string{}},
	{Kind: Audit, Code: "route_taxonomy_apply", Payload: []string{}},
	{Kind: Audit, Code: "route_torrent_add", Payload: []string{}},
	{Kind: Audit, Code: "route_torrent_create", Payload: []string{}},
	{Kind: Audit, Code: "route_torrent_migrate", Payload: []string{}},
	{Kind: Audit, Code: "route_torrent_pause", Payload: []string{}},
	{Kind: Audit, Code: "route_torrent_recheck", Payload: []string{}},
	{Kind: Audit, Code: "route_torrent_remove", Payload: []string{}},
	{Kind: Audit, Code: "route_torrent_resume", Payload: []string{}},
	{Kind: Audit, Code: "route_tracker_add", Payload: []string{}},
	{Kind: Audit, Code: "route_tracker_default", Payload: []string{}},
	{Kind: Audit, Code: "route_tracker_edit", Payload: []string{}},
	{Kind: Audit, Code: "route_tracker_remove", Payload: []string{}},
	{Kind: Audit, Code: "route_tracker_seed", Payload: []string{}},
	{Kind: Audit, Code: "setting_changed", Payload: []string{"key", "label", "value", "value_kind"}, Variant: "value_kind", Variants: []string{"plain", "secret", "cluster"}},
	{Kind: Audit, Code: "setting_reset", Payload: []string{"key", "label"}},
	{Kind: Audit, Code: "torrent_migrated", Payload: []string{"name", "source", "dest"}},
	{Kind: Audit, Code: "torrent_paused", Payload: []string{"name"}},
	{Kind: Audit, Code: "torrent_prioritized", Payload: []string{"name"}},
	{Kind: Audit, Code: "torrent_rechecked", Payload: []string{"name"}},
	{Kind: Audit, Code: "torrent_removed", Payload: []string{"name"}},
	{Kind: Audit, Code: "torrent_resumed", Payload: []string{"name"}},
	{Kind: Audit, Code: "torrent_seeded_from_tracker", Payload: []string{"name", "tracker"}},
	{Kind: Audit, Code: "torrent_unprioritized", Payload: []string{"name"}},
	{Kind: Audit, Code: "tracker_description_edited", Payload: []string{"tracker"}},
	{Kind: Audit, Code: "tracker_description_reset", Payload: []string{"tracker"}},
	{Kind: Audit, Code: "tracker_image_host_edited", Payload: []string{"tracker"}},
	{Kind: Audit, Code: "tracker_image_host_key_removed", Payload: []string{"tracker"}},
	{Kind: Audit, Code: "tracker_image_host_key_set", Payload: []string{"host", "tracker"}},
	{Kind: Audit, Code: "tracker_image_host_removed", Payload: []string{"tracker"}},
	{Kind: Audit, Code: "tracker_profile_edited", Payload: []string{"tracker"}},
	{Kind: Audit, Code: "tracker_profile_imported", Payload: []string{"tracker", "new", "renamed", "kind_set", "protected", "description", "image_host"}},
	{Kind: Audit, Code: "tracker_taxonomy_applied", Payload: []string{"tracker", "added", "renamed"}},
	{Kind: Audit, Code: "user_created", Payload: []string{"username"}},
	{Kind: Audit, Code: "user_disabled", Payload: []string{"user"}},
	{Kind: Audit, Code: "user_password_reset", Payload: []string{"user"}},
	{Kind: Audit, Code: "user_updated", Payload: []string{"username"}},
}
