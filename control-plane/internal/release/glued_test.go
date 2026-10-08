package release

import "testing"

// Merged tags (#187). "DDP5.1" lost TWO fields at once: the codec was not
// delimited ("ddp5") and the channel was split in half ("5 1" with no left boundary).
// The operator manually fixed every publication in the dashboard.
func TestParse_GluedTags(t *testing.T) {
	for _, tc := range []struct {
		name         string
		in           string
		wantAudio    string
		wantChannels string
	}{
		{"DDP5.1", "Movie.2020.1080p.WEB-DL.DDP5.1.H.264-GROUP", "DDP", "5.1"},
		{"DD+5.1", "Movie.2020.1080p.WEB-DL.DD+5.1.x264-GROUP", "DDP", "5.1"},
		{"AAC2.0", "Movie.2020.1080p.WEBRip.AAC2.0.x264-GROUP", "AAC", "2.0"},
		{"DTS5.1", "Movie.2020.1080p.BluRay.DTS5.1.x264-GROUP", "DTS", "5.1"},
		{"TrueHD7.1", "Movie.2020.2160p.BluRay.TrueHD7.1.Atmos-GROUP", "TRUEHD", "7.1"},
		{"FLAC2.0", "Show.2019.1080p.BluRay.FLAC2.0.x264-GROUP", "FLAC", "2.0"},

		// They already worked — guards against regression from the new normalization.
		{"separated by dot", "Movie.2020.1080p.BluRay.DTS-HD.MA.5.1.x264-GROUP", "DTS-HD", "5.1"},
		{"TrueHD separated", "Movie.2020.2160p.BluRay.TrueHD.7.1.Atmos-GROUP", "TRUEHD", "7.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if got.Audio != tc.wantAudio {
				t.Errorf("audio = %q, want %q", got.Audio, tc.wantAudio)
			}
			if got.Channels != tc.wantChannels {
				t.Errorf("channels = %q, want %q", got.Channels, tc.wantChannels)
			}
		})
	}
}

// The separation is restricted to audio codecs on purpose: breaking any
// letter→digit boundary would break the video codec, and digit→letter would destroy the resolution.
func TestParse_NormalizationDoesNotBreakCodecOrResolution(t *testing.T) {
	for _, tc := range []struct {
		name        string
		in          string
		wantEnc     string
		wantResName string
	}{
		{"x264 intact", "Movie.2020.1080p.BluRay.DD5.1.x264-GROUP", "x264", "1080p"},
		{"x265 with 10bit", "Movie.2020.2160p.BluRay.x265.10bit.DTS5.1-GROUP", "x265", "2160p"},
		{"H.264 dotted", "Movie.2020.1080p.WEB-DL.DDP5.1.H.264-GROUP", "H.264", "1080p"},
		{"h264 joined", "Movie.2020.720p.WEBRip.AAC2.0.h264-GROUP", "H.264", "720p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if got.Encoding != tc.wantEnc {
				t.Errorf("encoding = %q, want %q", got.Encoding, tc.wantEnc)
			}
			if got.Resolution != tc.wantResName {
				t.Errorf("resolution = %q, want %q", got.Resolution, tc.wantResName)
			}
		})
	}
}

// "dd" before "ddp" in the alternation would leave the "p" out of the codec and swap DDP for DD
// silently — the kind of error that only appears in the tracker catalog after being published.
func TestParse_DDPDoesNotBecomeDD(t *testing.T) {
	if got := Parse("Movie.2020.1080p.WEB-DL.DDP5.1.x264-GROUP").Audio; got != "DDP" {
		t.Errorf("audio = %q, want DDP (alternation must try the longest pattern first)", got)
	}
}
