package release

import "testing"

// The "same work" pairs come from a real catalog × the real name in a cluster: they are the ones
// the name match used to miss, and they are why the key exists (#37).
func TestKeyOf_SameWork(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"catalog × file, DD+ × DDP", "Brotherhood S02 1080p NF WEB-DL DD+ 5.1 AV1 DUAL-HAO",
			"Brotherhood.S02.1080p.NF.WEB-DL.DDP5.1.AV1.DUAL-HAO", true},
		{"word order and extension", "The Pursuit of Happyness 2006 1080p BluRay x264 DTS-HDWinG DUAL-wastaken",
			"The.Pursuit.of.Happyness.2006.BluRay.1080p.x264.DTS-HDWinG.DUAL-wastaken.mkv", true},
		{"colon and case", "Teenage Mutant Ninja Turtles: Out of the Shadows 2016 2160p PMTP WEB-DL DDP5.1 Atmos HDR10+ H.265 DUAL-EcK",
			"Teenage.Mutant.Ninja.Turtles.Out.Of.The.Shadows.2016.2160p.PMTP.WEB-DL.DDP5.1.Atmos.HDR10P.H.265.DUAL-EcK.mkv", true},
		{"another resolution is the same work", "Interstellar 2014 720p BluRay x264-GRP", "Interstellar.2014.2160p.UHD.BluRay.x265-OTHER", true},
		{"another codec is the same work", "Interstellar 2014 1080p BluRay x264-GRP", "Interstellar.2014.1080p.BluRay.x265-OTHER", true},
		{"accent on one side only", "Amélie 2001 1080p BluRay", "Amelie.2001.1080p.BluRay", true},
		{"year missing on one side", "Brotherhood 2015 S01 1080p WEB-DL", "Brotherhood.S01.1080p.WEB-DL", true},
		{"different years keep remakes apart", "Dune 1984 1080p BluRay x264", "Dune.2021.1080p.BluRay.x264", false},
		{"different seasons", "Brotherhood S01 1080p WEB-DL", "Brotherhood.S02.1080p.WEB-DL", false},
		{"pack × episode", "Brotherhood S01 1080p WEB-DL", "Brotherhood.S01E03.1080p.WEB-DL", false},
		{"different parts (#172)", "The Batman Saga - Part 1 2020 1080p", "The Batman Saga - Part 2 2020 1080p", false},
		{"different titles", "Interstellar 2014 1080p BluRay", "Inception 2010 1080p BluRay", false},
	}
	for _, c := range cases {
		ka, kb := KeyOf(c.a), KeyOf(c.b)
		if got := ka.SameWork(kb); got != c.want {
			t.Errorf("%s: SameWork = %v, want %v\n  %q → %+v\n  %q → %+v", c.name, got, c.want, c.a, ka, c.b, kb)
		}
		if got := kb.SameWork(ka); got != c.want {
			t.Errorf("%s: SameWork is not symmetric", c.name)
		}
	}
}

// Without a recognizable title, the key is the whole normalized name — never empty, which would
// lump every name the parser doesn't understand into a single work.
func TestKeyOf_NeverEmpty(t *testing.T) {
	for _, n := range []string{"1080p", "x264-GRP"} {
		if KeyOf(n).Title == "" {
			t.Errorf("KeyOf(%q).Title is empty", n)
		}
	}
	if (TitleKey{}).SameWork(TitleKey{}) {
		t.Error("two empty keys can't be the same work")
	}
}
