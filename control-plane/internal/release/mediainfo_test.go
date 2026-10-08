package release

import "testing"

// realistic (trimmed) mediainfo of a file whose NAME only has title+year — the case
// "A Bruxa 2015" reported: TMDB/mediainfo OK, but the name came out without enrichment.
const sampleMediaInfo = `
General
Complete name  : A Bruxa 2015.mkv
Format         : Matroska

Video
ID             : 1
Format         : AVC
Format profile : High@L4
Width          : 1 920 pixels
Height         : 1 080 pixels
Writing library: x264 core 148

Audio
ID             : 2
Format         : E-AC-3
Channel(s)     : 6 channels
Language       : English

Audio #2
ID             : 3
Format         : AAC
Channel(s)     : 2 channels

Text
Format         : UTF-8
`

func TestEnrichFromMediaInfo_BareName(t *testing.T) {
	info := Parse("A Bruxa 2015")
	// Before enrichment: raw name.
	if info.StandardName != "A Bruxa 2015-NOGROUP" {
		t.Fatalf("baseline = %q, want \"A Bruxa 2015-NOGROUP\"", info.StandardName)
	}
	EnrichFromMediaInfo(&info, sampleMediaInfo)

	if info.Resolution != "1080p" {
		t.Errorf("resolution = %q, want 1080p", info.Resolution)
	}
	if info.Encoding != "x264" {
		t.Errorf("video codec = %q, want x264 (Writing library)", info.Encoding)
	}
	if info.Audio != "DDP" || info.Channels != "5.1" {
		t.Errorf("audio = %q%q, want DDP5.1 (E-AC-3 / 6ch, 1st track)", info.Audio, info.Channels)
	}
	want := "A Bruxa 2015 1080p DDP5.1 x264-NOGROUP"
	if info.StandardName != want {
		t.Errorf("StandardName = %q, want %q", info.StandardName, want)
	}
}

// The file name takes priority: if it already has resolution/codec, mediainfo does not
// overwrite (but supplements what is missing).
func TestEnrichFromMediaInfo_NameWins(t *testing.T) {
	info := Parse("A Bruxa 2015 720p BluRay x265-GRP")
	EnrichFromMediaInfo(&info, sampleMediaInfo)
	if info.Resolution != "720p" {
		t.Errorf("resolution = %q, want 720p (from name, not 1080p from mediainfo)", info.Resolution)
	}
	if info.Encoding != "x265" {
		t.Errorf("codec = %q, want x265 (from name)", info.Encoding)
	}
	// BluRay came from the name; audio (absent from the name) came from mediainfo.
	if info.Source != "BluRay" {
		t.Errorf("source = %q, want BluRay (from name — mediainfo does not know)", info.Source)
	}
	if info.Audio != "DDP" {
		t.Errorf("audio = %q, want DDP (supplemented from mediainfo)", info.Audio)
	}
}
