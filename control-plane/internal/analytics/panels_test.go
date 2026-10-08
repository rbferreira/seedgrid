package analytics

import "testing"

func tt(name string, size int64, node string, up, down int64, seeds int) Torrent {
	t := Torrent{Name: name, SizeBytes: size, NodeID: node, Uploaded: up, Downloaded: down, NumSeeds: seeds}
	if down > 0 {
		t.Ratio = float64(up) / float64(down)
	}
	return t
}

// The distinction that the manual cleanup on 06/09/2026 showed is what matters: a copy
// IDENTICAL on two Nodes is pure waste (trivial decision); the same title at
// a different resolution is NOT — the tracker accepts both and both yield ratio.
func TestDuplicates_IdenticalVsDifferentResolution(t *testing.T) {
	all := []Torrent{
		tt("Death.Inc..S01.1080p.NF.WEB-DL.DDP5.1.Atmos.H.264.DUAL-BiOMA", 11_000, "n2", 0, 0, 4),
		tt("Death.Inc..S01.1080p.NF.WEB-DL.DDP5.1.Atmos.H.264.DUAL-BiOMA", 11_000, "n3", 0, 0, 0),
		tt("Gladiator.II.2024.2160p.AMZN.WEB-DL.DDP5.1.HDR.H.265.DUAL-C76.mkv", 16_000, "n3", 0, 0, 3),
		tt("Gladiator.II.2024.1080p.AMZN.WEB-DL.AAC2.0.H.265.DUAL-nitrato.mkv", 2_800, "n3", 0, 0, 4),
	}
	got := duplicates(all)
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got))
	}
	byTitle := map[string]DuplicateSet{}
	for _, d := range got {
		byTitle[d.Title] = d
	}
	death, ok := byTitle["Death Inc"]
	if !ok {
		for k, v := range byTitle {
			if len(k) >= 5 && k[:5] == "Death" {
				death, ok = v, true
			}
		}
	}
	if !ok {
		t.Fatalf("group Death Inc not found; titles: %v", byTitle)
	}
	if !death.Identical {
		t.Error("same release and same size on different Nodes must be marked Identical")
	}
	for tit, d := range byTitle {
		if tit != death.Title && d.Identical {
			t.Errorf("%q: different resolution is NOT an identical copy", tit)
		}
	}
}

// Recoverable space counts everything except the LARGEST copy — dropping the largest and keeping the
// smaller one would save more disk, but it's the opposite decision of what the operator wants.
func TestDuplicates_ReclaimIgnoresLargest(t *testing.T) {
	all := []Torrent{
		tt("Filme.2020.2160p.WEB-DL.x265-A.mkv", 16_000, "n1", 0, 0, 1),
		tt("Filme.2020.1080p.WEB-DL.x265-B.mkv", 3_000, "n1", 0, 0, 1),
		tt("Filme.2020.720p.WEB-DL.x265-C.mkv", 1_000, "n1", 0, 0, 1),
	}
	got := duplicates(all)
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	if got[0].ReclaimBytes != 4_000 {
		t.Fatalf("reclaim = %d, expected 4000 (3000+1000, excluding the largest)", got[0].ReclaimBytes)
	}
	if got[0].Copies[0].SizeBytes != 16_000 {
		t.Fatal("the first copy must be the largest — it is the one that stays")
	}
}

// Different episodes of the same series are NOT duplicates. Without S/E in the key, an entire
// series would become a giant "duplicate group" and the panel would be useless.
func TestDuplicates_DifferentEpisodesDoNotGroup(t *testing.T) {
	all := []Torrent{
		tt("Silo.S03E01.1080p.ATVP.WEB-DL.DUAL-BiOMA.mkv", 4_000, "n1", 0, 0, 1),
		tt("Silo.S03E02.1080p.ATVP.WEB-DL.DUAL-BiOMA.mkv", 4_000, "n1", 0, 0, 1),
	}
	if got := duplicates(all); len(got) != 0 {
		t.Fatalf("distinct episodes are not duplicates; got %d group(s)", len(got))
	}
}

// "Never seeded" and "never seeded AND no one else has it" are DIFFERENT decisions: in the
// latter the content died in the swarm; in the former there just hasn't been demand yet.
func TestIdle_SeparatesDeadSwarm(t *testing.T) {
	all := []Torrent{
		tt("A.2020.1080p-x.mkv", 5_000, "n1", 0, 100, 7),   // stopped, but there is a swarm
		tt("B.2020.1080p-y.mkv", 9_000, "n1", 0, 100, 0),   // stopped and no one has it
		tt("C.2020.1080p-z.mkv", 1_000, "n1", 500, 100, 3), // yielded: out
	}
	s := markIdle(all)
	if s.NeverUploaded != 2 || s.NeverUploadedBytes != 14_000 {
		t.Fatalf("never uploaded = %d/%d, expected 2/14000", s.NeverUploaded, s.NeverUploadedBytes)
	}
	if s.DeadSwarm != 1 || s.DeadSwarmBytes != 9_000 {
		t.Fatalf("dead swarm = %d/%d, expected 1/9000", s.DeadSwarm, s.DeadSwarmBytes)
	}
	// The flag must travel IN the torrent: it's what links the screen counter to the item (#169).
	if !all[1].DeadSwarm || !all[1].NeverUploaded {
		t.Error("the dead-swarm one must be marked in both flags")
	}
	if all[2].NeverUploaded {
		t.Error("the one that yielded cannot be marked as stopped")
	}
}

// TestCounterHasList is the invariant that #169 asks for: every screen number needs to have
// a corresponding list, and both come from the SAME source. Before, the counter came from the entire
// collection and the list came truncated at 20 — nothing guaranteed they matched.
func TestCounterHasList(t *testing.T) {
	all := []Torrent{
		tt("A.2020.1080p-x.mkv", 5_000, "n1", 0, 100, 7),
		tt("B.2020.1080p-y.mkv", 9_000, "n1", 0, 100, 0),
		tt("C.2020.1080p-z.mkv", 1_000, "n1", 500, 100, 3),
		tt("D.2021.1080p-w.mkv", 2_000, "n2", 0, 50, 0),
	}
	s := markIdle(all)

	var never, dead int
	for _, x := range all {
		if x.NeverUploaded {
			never++
		}
		if x.DeadSwarm {
			dead++
		}
	}
	if never != s.NeverUploaded {
		t.Fatalf("never-uploaded counter = %d but the list has %d", s.NeverUploaded, never)
	}
	if dead != s.DeadSwarm {
		t.Fatalf("dead-swarm counter = %d but the list has %d", s.DeadSwarm, dead)
	}
}

// NULL status is UNKNOWN and can never become an accusation of a dead torrent — it's the same
// semantics as storage_ok and incoming_peers.
func TestTrackerHealth_UnknownNotDead(t *testing.T) {
	ok, dead := 2, 5
	all := []Torrent{
		{Name: "vivo", TrackerStatus: &ok},
		{Name: "morto", TrackerStatus: &dead, TrackerMessage: "Torrent has been deleted."},
		{Name: "not sampled"}, // TrackerStatus nil
	}
	unknown := markTracker(all)
	if !all[1].TrackerDead {
		t.Error("the one rejected by the tracker must be marked")
	}
	if all[0].TrackerDead || all[2].TrackerDead {
		t.Error("only the rejected one can be marked — unknown never becomes an accusation")
	}
	if unknown != 1 {
		t.Fatalf("unknowns = %d, expected 1", unknown)
	}
}

// Recoverable by duplication counts on the Node of EACH excess copy — that's where the disk
// would be freed, and that's the question from whoever has a full disk.
func TestNodeUsage_ReclaimGoesToCopyNode(t *testing.T) {
	all := []Torrent{
		tt("Filme.2020.2160p-A.mkv", 16_000, "n1", 0, 0, 1),
		tt("Filme.2020.1080p-B.mkv", 3_000, "n2", 0, 0, 1),
	}
	us := nodeUsage(all, duplicates(all))
	by := map[string]NodeUsage{}
	for _, u := range us {
		by[u.NodeID] = u
	}
	if by["n2"].ReclaimBytes != 3_000 {
		t.Fatalf("n2 reclaim = %d, expected 3000 (the extra copy is ON IT)", by["n2"].ReclaimBytes)
	}
	if by["n1"].ReclaimBytes != 0 {
		t.Fatalf("n1 reclaim = %d, expected 0 (the copy that stays is on it)", by["n1"].ReclaimBytes)
	}
}

// Phase 1 and Phase 2 of the same title are different works and CANNOT fall into the same group:
// the smaller one was marked as an excess copy, suggesting freeing 3.25 GB that aren't actually free (#172).
func TestDuplicates_PartIsNotCopy(t *testing.T) {
	all := []Torrent{
		{ID: "1", Name: "A Saga do Batman - Fase 1", SizeBytes: 8_630_000_000},
		{ID: "2", Name: "A Saga do Batman - Fase 2", SizeBytes: 3_250_000_000},
	}
	if got := duplicates(all); len(got) != 0 {
		t.Fatalf("different parts became %d duplicate group(s): %+v", len(got), got)
	}
	markDuplicates(all, duplicates(all))
	for _, tr := range all {
		if tr.DuplicateExtra {
			t.Errorf("%q was marked as an extra copy", tr.Name)
		}
	}
}

// The same part on two Nodes remains a duplicate — the fix cannot blind the panel.
func TestDuplicates_SamePartStillGroups(t *testing.T) {
	all := []Torrent{
		{ID: "1", Name: "A Saga do Batman - Fase 1", NodeID: "a", SizeBytes: 8_630_000_000},
		{ID: "2", Name: "A Saga do Batman - Fase 1", NodeID: "b", SizeBytes: 8_630_000_000},
	}
	got := duplicates(all)
	if len(got) != 1 {
		t.Fatalf("want 1 group, got %d", len(got))
	}
	if !got[0].Identical {
		t.Error("same name and same size should be identical (pure waste)")
	}
	if got[0].Part != "Fase 1" {
		t.Errorf("Part = %q, want %q", got[0].Part, "Fase 1")
	}
}

// The "copy of:" label needs to say WHICH part, otherwise it points to a title that doesn't
// exist alone in the collection.
func TestMarkDuplicates_LabelIncludesPart(t *testing.T) {
	all := []Torrent{
		{ID: "1", Name: "Duna - Parte 2 (2024) 2160p", SizeBytes: 60_000_000_000},
		{ID: "2", Name: "Duna - Parte 2 (2024) 1080p", SizeBytes: 12_000_000_000},
	}
	markDuplicates(all, duplicates(all))
	if all[1].DuplicateOf != "Duna (2024) — Parte 2" {
		t.Errorf("DuplicateOf = %q, want %q", all[1].DuplicateOf, "Duna (2024) — Parte 2")
	}
}
