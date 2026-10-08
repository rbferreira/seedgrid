package release

import (
	"regexp"
	"strconv"
	"strings"
)

// This file enriches the Info (and therefore the StandardName) with the TECHNICAL data that
// only exists in the file — resolution, video codec, audio codec+channels — read from
// the mediainfo text. This is what solves the "A Bruxa 2015" case (name with only title+year):
// the mediainfo has 1080p/x264/DDP5.1, so the name becomes "A Bruxa 2015 1080p x264 DDP5.1".
// The SOURCE (BluRay/WEB-DL) does NOT come from the mediainfo — it still depends on the filename.

// EnrichFromMediaInfo fills the empty technical fields of the Info from the mediainfo
// and recomputes the StandardName. It only fills what was missing (the filename has
// priority — it may carry source/flags that the mediainfo doesn't know about).
func EnrichFromMediaInfo(info *Info, mediaInfo string) {
	if info == nil || strings.TrimSpace(mediaInfo) == "" {
		return
	}
	m := parseMediaInfo(mediaInfo)
	if info.Resolution == "" && m.resolution != "" {
		info.Resolution = m.resolution
	}
	if info.Encoding == "" && m.videoCodec != "" {
		info.Encoding = m.videoCodec
	}
	if info.Audio == "" && m.audio != "" {
		info.Audio = m.audio
		if info.Channels == "" {
			info.Channels = m.channels
		}
	}
	info.StandardName = standardName(*info)
}

type mediaFields struct {
	resolution string
	videoCodec string
	audio      string
	channels   string
}

// parseMediaInfo reads the standard mediainfo text (General/Video/Audio/… sections with
// "Key : Value") and extracts the resolution (from the 1st video track), video codec, and the
// codec+channels of the 1st audio track.
func parseMediaInfo(text string) mediaFields {
	var vWidth, vHeight int
	var vFormat, vLibrary, aFormat, aChannels string
	section := ""
	audioIndex := 0

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 { // section header: "General", "Video", "Audio", "Audio #2", "Text"…
			section = strings.ToLower(strings.Fields(line)[0])
			if section == "audio" {
				audioIndex++
			}
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])
		switch section {
		case "video":
			switch {
			case key == "width" && vWidth == 0:
				vWidth = firstInt(val)
			case key == "height" && vHeight == 0:
				vHeight = firstInt(val)
			case key == "format" && vFormat == "":
				vFormat = val
			case strings.HasPrefix(key, "writing library") && vLibrary == "":
				vLibrary = val
			}
		case "audio":
			if audioIndex == 1 { // only the first audio track
				switch {
				case key == "format" && aFormat == "":
					aFormat = val
				case (key == "channel(s)" || key == "channels") && aChannels == "":
					aChannels = val
				}
			}
		}
	}

	return mediaFields{
		resolution: resFromDims(vWidth, vHeight),
		videoCodec: videoCodecLabel(vFormat, vLibrary),
		audio:      audioLabel(aFormat),
		channels:   channelsLabel(firstInt(aChannels)),
	}
}

var reLeadingInt = regexp.MustCompile(`^\d+`)

// firstInt extracts the first integer from val, ignoring internal spaces ("1 080
// pixels" → 1080; "6 channels" → 6).
func firstInt(val string) int {
	s := strings.ReplaceAll(val, " ", "")
	m := reLeadingInt.FindString(s)
	if m == "" {
		return 0
	}
	n, _ := strconv.Atoi(m)
	return n
}

// resFromDims maps width/height to the resolution label (uses width OR height —
// covers content with black bars, where the active height is smaller).
func resFromDims(w, h int) string {
	switch {
	case w >= 7680 || h >= 4320:
		return "4320p"
	case w >= 3840 || h >= 2160:
		return "2160p"
	case w >= 1920 || h >= 1080:
		return "1080p"
	case w >= 1280 || h >= 720:
		return "720p"
	case w >= 640 || h >= 480:
		return "480p"
	}
	return ""
}

// videoCodecLabel picks the video codec label. The "Writing library" distinguishes a
// re-encode (x264/x265) from the raw format (H.264/H.265 when only the Format is present).
func videoCodecLabel(format, library string) string {
	lib := strings.ToLower(library)
	switch {
	case strings.Contains(lib, "x264"):
		return "x264"
	case strings.Contains(lib, "x265"):
		return "x265"
	}
	f := strings.ToLower(format)
	switch {
	case strings.Contains(f, "hevc") || strings.Contains(f, "h.265") || strings.Contains(f, "h265"):
		return "H.265"
	case strings.Contains(f, "avc") || strings.Contains(f, "h.264") || strings.Contains(f, "h264"):
		return "H.264"
	case strings.Contains(f, "xvid") || strings.Contains(f, "mpeg-4 visual"):
		return "XviD"
	case strings.Contains(f, "vc-1"):
		return "VC-1"
	}
	return ""
}

// audioLabel maps the audio track Format to the tracker label (E-AC-3 before
// AC-3, since one is a substring of the other).
func audioLabel(format string) string {
	f := strings.ToLower(format)
	switch {
	case strings.Contains(f, "e-ac-3") || strings.Contains(f, "eac3") || strings.Contains(f, "e-ac3"):
		return "DDP"
	case strings.Contains(f, "ac-3") || strings.Contains(f, "ac3"):
		return "DD"
	case strings.Contains(f, "truehd"):
		return "TRUEHD"
	case strings.Contains(f, "dts"):
		return "DTS"
	case strings.Contains(f, "aac"):
		return "AAC"
	case strings.Contains(f, "flac"):
		return "FLAC"
	case strings.Contains(f, "opus"):
		return "Opus"
	}
	return ""
}

// channelsLabel converts the channel count to the label (6 → 5.1). "" for counts
// outside the tracker's known set.
func channelsLabel(n int) string {
	switch n {
	case 1:
		return "1.0"
	case 2:
		return "2.0"
	case 6:
		return "5.1"
	case 7:
		return "6.1"
	case 8:
		return "7.1"
	case 10:
		return "9.1"
	}
	return ""
}
