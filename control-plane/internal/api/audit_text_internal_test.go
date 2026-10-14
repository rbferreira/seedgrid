package api

import "testing"

func TestAgentUpdateAudit(t *testing.T) {
	params, text := agentUpdateAudit("seedgrid-node-02", "v0.1.0-4", "v0.1.0-5")
	if want := "Requested an Agent update on Node «seedgrid-node-02» (v0.1.0-4 → v0.1.0-5)"; text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
	if params["node"] != "seedgrid-node-02" || params["from"] != "v0.1.0-4" || params["to"] != "v0.1.0-5" {
		t.Errorf("params = %v", params)
	}
	// An Agent that never reported a version: the entry still reads whole.
	if params, text := agentUpdateAudit("n1", "", "v1"); params["from"] != "?" || text != "Requested an Agent update on Node «n1» (? → v1)" {
		t.Errorf("unknown version: %v %q", params, text)
	}
}

func TestSeedAudit(t *testing.T) {
	params, text := seedAudit("debian-13.1.0-amd64-netinst.iso", "Public Linux")
	if want := "Seeded torrent «debian-13.1.0-amd64-netinst.iso» from tracker «Public Linux»"; text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
	if params["name"] != "debian-13.1.0-amd64-netinst.iso" || params["tracker"] != "Public Linux" {
		t.Errorf("params = %v", params)
	}
	// The tracker name cannot forge the rest of the line with the quotes that delimit it.
	if _, text := seedAudit("x.iso", "False» and then «Admin"); text != "Seeded torrent «x.iso» from tracker «False and then Admin»" {
		t.Errorf("quotes: %q", text)
	}
	// No torrent name yet: the sentence does not show empty quotes.
	if _, text := seedAudit("", "Public Linux"); text != "Seeded a torrent from tracker «Public Linux»" {
		t.Errorf("no name: %q", text)
	}
}
