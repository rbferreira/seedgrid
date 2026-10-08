package release

import (
	"reflect"
	"testing"
)

func TestParseAudioLangs(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"Title.2023.1080p.WEB-DL.DUAL.x264-GROUP", []string{"PT-BR DUB", "Original"}},
		{"Movie.2020.BluRay.720p.DUBLADO-XPTO", []string{"PT-BR DUB"}},
		{"Thing.1080p.NACIONAL.x265-GRP", []string{"PT-BR DUB"}},
		{"Show.S01.1080p.MULTI-XYZ", []string{"MULTI"}},
		{"Movie.MULTI2.1080p", []string{"MULTI"}},
		{"Anime.1080p.SUBBED-Group", nil},           // subtitle, not audio
		{"Movie.2023.1080p.WEB-DL.x264-GROUP", nil}, // no marker → nothing (don't invent)
	}
	for _, tc := range cases {
		got := ParseAudioLangs(tc.name)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseAudioLangs(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseSubtitleLangs(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"Title.2023.1080p.WEB-DL.LEG.x264-GROUP", []string{"PT-BR LEG"}},
		{"Movie.LEGENDADO.720p", []string{"PT-BR LEG"}},
		{"Movie.VOSTFR.1080p", []string{"FR LEG"}},
		{"Show.S01.SDH.1080p", []string{"SDH"}},
		{"Anime.SUBBED.1080p", []string{"EN SUB"}},
		{"Movie.LEG.SUBBED.1080p", []string{"PT-BR LEG"}}, // PT-BR overrides EN
		{"Movie.1080p.x264-GRP", nil},
	}
	for _, tc := range cases {
		got := ParseSubtitleLangs(tc.name)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseSubtitleLangs(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseResolution(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Movie.2023.1080p.WEB-DL.x264", "1080p"},
		{"Movie.2160p.HDR", "2160p"},
		{"Movie.4K.HDR", "2160p"},
		{"Movie.720p.HDTV", "720p"},
		{"Movie.WEB-DL.x264", ""},
	}
	for _, tc := range cases {
		if got := ParseResolution(tc.name); got != tc.want {
			t.Errorf("ParseResolution(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
