package api

import "github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"

// agentUpdateAudit is the audit entry for an Agent update request: which Node and which versions.
// An Agent that never reported its version shows as "?".
func agentUpdateAudit(node, from, to string) (map[string]any, string) {
	if from == "" {
		from = "?"
	}
	return map[string]any{"node": node, "from": from, "to": to},
		"Requested an Agent update on Node «" + node + "» (" + from + " → " + to + ")"
}

// seedAudit is the audit entry for seeding a torrent from a tracker: which torrent, from which
// tracker. The tracker name goes through AuditLabel, which drops the quotes that delimit it and
// never lets a passkey-like run through.
func seedAudit(name, trackerName string) (map[string]any, string) {
	label := trackerprofile.AuditLabel(trackerName)
	params := map[string]any{"name": name, "tracker": label}
	if name == "" {
		return params, "Seeded a torrent from tracker «" + label + "»"
	}
	return params, "Seeded torrent «" + name + "» from tracker «" + label + "»"
}
