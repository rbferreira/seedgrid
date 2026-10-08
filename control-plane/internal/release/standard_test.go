package release

import "testing"

func TestStandardNameIdiocracy(t *testing.T) {
	// Tracker format: spaces, source before video codec (was the cause of the postponed
	// of the actual Idiocracy — name was "Idiocracy.2006.1080p.x264.BluRay.2.0-NOGROUP").
	got := Parse("Idiocracy (2006) {imdb-tt0387808} [Bluray-1080p][MP2 2.0][x264].mkv")
	want := "Idiocracy 2006 1080p BluRay 2.0 x264-NOGROUP"
	if got.StandardName != want {
		t.Errorf("standard_name = %q, want %q\n(source=%q encoding=%q channels=%q audio=%q)",
			got.StandardName, want, got.Source, got.Encoding, got.Channels, got.Audio)
	}
}

func TestStandardNameSceneMovie(t *testing.T) {
	// Tracker order: Title Year Resolution Streaming Source Audio+Channels VideoCodec-GROUP
	// (validated with approved upload "Better Watch Out 2017 1080p AMZN WEB-DL DDP5.1 H.264 DUAL-cza").
	// Audio+channels joined: DTS+5.1 → "DTS5.1".
	got := Parse("Some.Movie.2023.2160p.AMZN.WEB-DL.DTS.5.1.x265-ABC")
	want := "Some Movie 2023 2160p AMZN WEB-DL DTS5.1 x265-ABC"
	if got.StandardName != want {
		t.Errorf("standard_name = %q, want %q\n(res=%q enc=%q sub=%q src=%q audio=%q ch=%q)",
			got.StandardName, want, got.Resolution, got.Encoding, got.Subsource, got.Source, got.Audio, got.Channels)
	}
}

func TestStandardNameSceneDualApproved(t *testing.T) {
	// Reconstructs exactly the example approved by the tracker staff.
	got := Parse("Better.Watch.Out.2017.1080p.AMZN.WEB-DL.DDP.5.1.H.264.DUAL-cza")
	want := "Better Watch Out 2017 1080p AMZN WEB-DL DDP5.1 H.264 DUAL-cza"
	if got.StandardName != want {
		t.Errorf("standard_name = %q, want %q\n(sub=%q src=%q audio=%q ch=%q enc=%q flags=%v)",
			got.StandardName, want, got.Subsource, got.Source, got.Audio, got.Channels, got.Encoding, got.Flags)
	}
}

func TestStandardNameTVEpisode(t *testing.T) {
	got := Parse("The.Show.S02E05.1080p.WEB-DL.x264-XYZ")
	want := "The Show S02E05 1080p WEB-DL x264-XYZ"
	if got.StandardName != want {
		t.Errorf("standard_name = %q, want %q", got.StandardName, want)
	}
}

func TestStandardNameFlags(t *testing.T) {
	got := Parse("Film.2019.1080p.BluRay.REMUX.PROPER.x264-GRP")
	// REMUX + PROPER flags go between source and audio.
	if got.Source != "BluRay" || got.Encoding != "x264" {
		t.Fatalf("source/encoding = %q/%q", got.Source, got.Encoding)
	}
	hasRemux, hasProper := false, false
	for _, f := range got.Flags {
		if f == "REMUX" {
			hasRemux = true
		}
		if f == "PROPER" {
			hasProper = true
		}
	}
	if !hasRemux || !hasProper {
		t.Errorf("flags = %v, want to contain REMUX and PROPER", got.Flags)
	}
}
