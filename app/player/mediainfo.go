package player

import "fmt"

// infoRow is one label/value pair in the info overlay.
type infoRow struct {
	label string
	value string
}

// infoSection is a titled group of rows in the info overlay ("File", "Video",
// "Audio", "Subtitles").
type infoSection struct {
	title string
	rows  []infoRow
}

// mediaInfo is everything the info overlay shows about the current file,
// collected from mpv in one place (see Player.mediaInfo) so the formatting below
// stays mpv-free and testable. Zero values mean "mpv didn't report it".
type mediaInfo struct {
	path     string // what was loaded: absolute path, relative path, or URL
	filename string // mpv's own file name, used when path carries none
	duration float64
	fileSize int64

	width, height int64
	fps           float64
	videoBitrate  float64

	audioBitrate    float64
	audioChannels   string
	audioSampleRate float64

	tracks []trackInfo
}

// dash returns "—" for empty strings so unavailable fields render uniformly.
func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// resolution formats the selected video's frame size and rate.
func (m mediaInfo) resolution() string {
	if m.width <= 0 || m.height <= 0 {
		return "—"
	}
	res := fmt.Sprintf("%d×%d", m.width, m.height)
	if m.fps > 0 {
		res += fmt.Sprintf(" @ %.3g fps", m.fps)
	}
	return res
}

// audioLayout formats the selected audio stream's channel count and sample
// rate, empty when mpv reported neither.
func (m mediaInfo) audioLayout() string {
	switch {
	case m.audioChannels != "" && m.audioSampleRate > 0:
		return fmt.Sprintf("%s ch, %s", m.audioChannels, humanRate(m.audioSampleRate))
	case m.audioChannels != "":
		return fmt.Sprintf("%s ch", m.audioChannels)
	case m.audioSampleRate > 0:
		return humanRate(m.audioSampleRate)
	}
	return ""
}

// infoSections groups the media details the way the overlay draws them: the
// file itself, then one section per track kind. Every kind gets a section even
// with no tracks, so "no subtitles" is visible rather than absent.
func infoSections(m mediaInfo) []infoSection {
	dir, name := splitPath(m.path)
	if name == "" {
		name = m.filename
	}
	size := "—"
	if m.fileSize > 0 {
		size = humanBytes(m.fileSize)
	}

	file := infoSection{"File", []infoRow{
		{"Name", dash(name)},
		{"Folder", dash(dir)},
		{"Duration", humanDuration(m.duration)},
		{"Size", size},
	}}

	video := infoSection{"Video", append(
		trackRows(tracksOfKind(m.tracks, "video")),
		infoRow{"Resolution", m.resolution()},
		infoRow{"Bitrate", humanBitrate(m.videoBitrate)},
	)}

	audioRows := trackRows(tracksOfKind(m.tracks, "audio"))
	if layout := m.audioLayout(); layout != "" {
		audioRows = append(audioRows, infoRow{"Channels", layout})
	}
	audio := infoSection{"Audio", append(audioRows, infoRow{"Bitrate", humanBitrate(m.audioBitrate)})}

	subs := infoSection{"Subtitles", trackRows(tracksOfKind(m.tracks, "sub"))}

	return []infoSection{file, video, audio, subs}
}

// trackRows lists one row per track, so a file with several audio or subtitle
// streams shows all of them rather than only the one playing. No tracks of that
// kind is stated outright — an empty section would read as missing data.
func trackRows(tracks []trackInfo) []infoRow {
	if len(tracks) == 0 {
		return []infoRow{{"Tracks", "none"}}
	}
	rows := make([]infoRow, 0, len(tracks))
	for _, t := range tracks {
		rows = append(rows, infoRow{t.rowLabel(), dash(t.describe())})
	}
	return rows
}
