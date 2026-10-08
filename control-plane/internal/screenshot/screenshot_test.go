package screenshot

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestBuildBBCode(t *testing.T) {
	tests := []struct {
		name  string
		shots []Shot
		want  string
	}{
		{name: "empty", shots: nil, want: ""},
		{
			name:  "a screenshot (clickable thumbnail)",
			shots: []Shot{{Full: "https://i.ibb.co/a.jpg", Thumb: "https://i.ibb.co/at.jpg"}},
			want:  "[b]Screenshots[/b]\n[center]\n[url=https://i.ibb.co/a.jpg][img]https://i.ibb.co/at.jpg[/img][/url]\n[/center]",
		},
		{
			name: "multiple, centered",
			shots: []Shot{
				{Full: "https://i.ibb.co/a.jpg", Thumb: "https://i.ibb.co/at.jpg"},
				{Full: "https://i.ibb.co/b.jpg", Thumb: "https://i.ibb.co/bt.jpg"},
			},
			want: "[b]Screenshots[/b]\n[center]\n[url=https://i.ibb.co/a.jpg][img]https://i.ibb.co/at.jpg[/img][/url]\n[url=https://i.ibb.co/b.jpg][img]https://i.ibb.co/bt.jpg[/img][/url]\n[/center]",
		},
		{
			name:  "no thumbnail falls back to full image",
			shots: []Shot{{Full: "https://i.ibb.co/a.jpg", Thumb: ""}},
			want:  "[b]Screenshots[/b]\n[center]\n[url=https://i.ibb.co/a.jpg][img]https://i.ibb.co/a.jpg[/img][/url]\n[/center]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildBBCode(tt.shots); got != tt.want {
				t.Errorf("BuildBBCode(%v) = %q, want %q", tt.shots, got, tt.want)
			}
		})
	}
}

func TestDecodeFrames_Success(t *testing.T) {
	f1 := base64.StdEncoding.EncodeToString([]byte("frame-one"))
	f2 := base64.StdEncoding.EncodeToString([]byte("frame-two"))
	raw := json.RawMessage(`{"frames":["` + f1 + `","` + f2 + `"]}`)

	frames, err := decodeFrames(raw)
	if err != nil {
		t.Fatalf("decodeFrames() error = %v", err)
	}
	if len(frames) != 2 || string(frames[0]) != "frame-one" || string(frames[1]) != "frame-two" {
		t.Fatalf("decodeFrames() = %v, want [frame-one frame-two]", frames)
	}
}

func TestDecodeFrames_SkipsInvalidBase64(t *testing.T) {
	good := base64.StdEncoding.EncodeToString([]byte("ok-frame"))
	raw := json.RawMessage(`{"frames":["not-base64!!","` + good + `"]}`)

	frames, err := decodeFrames(raw)
	if err != nil {
		t.Fatalf("decodeFrames() error = %v", err)
	}
	if len(frames) != 1 || string(frames[0]) != "ok-frame" {
		t.Fatalf("decodeFrames() = %v, want [ok-frame]", frames)
	}
}

func TestDecodeFrames_NoValidFrames(t *testing.T) {
	raw := json.RawMessage(`{"frames":[]}`)
	if _, err := decodeFrames(raw); err == nil {
		t.Fatal("expected error with empty frames, got nil")
	}
}

func TestDecodeFrames_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not json`)
	if _, err := decodeFrames(raw); err == nil {
		t.Fatal("expected error with invalid JSON, got nil")
	}
}
