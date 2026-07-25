package player

import (
	"fmt"
	"math"
)

// trackInfo is one entry from mpv's track-list, normalized for the menu.
type trackInfo struct {
	id       int64
	kind     string // "audio", "video", "sub"
	title    string
	lang     string
	codec    string
	selected bool
}

// parseTrackList converts mpv's track-list node ([]any of map[string]any)
// into typed tracks. Returns nil for nil/unexpected input.
func parseTrackList(v any) []trackInfo {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]trackInfo, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t := trackInfo{
			kind:  asString(m["type"]),
			title: asString(m["title"]),
			lang:  asString(m["lang"]),
			codec: asString(m["codec"]),
		}
		t.id = asInt64(m["id"])
		if b, ok := m["selected"].(bool); ok {
			t.selected = b
		}
		out = append(out, t)
	}
	return out
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}

// label renders a track for a menu row: "id: title|lang (codec)".
func (t trackInfo) label() string {
	name := t.title
	if name == "" {
		name = t.lang
	}
	switch {
	case name != "" && t.codec != "":
		return fmt.Sprintf("%d: %s (%s)", t.id, name, t.codec)
	case name != "":
		return fmt.Sprintf("%d: %s", t.id, name)
	default:
		return fmt.Sprintf("%d", t.id)
	}
}

// zoomToVideoZoom maps a user-facing zoom factor (0.5×, 1×, 1.5×, 2×) to
// mpv's video-zoom, which is a log2 scale (1× -> 0, 2× -> 1, 0.5× -> -1).
func zoomToVideoZoom(factor float64) float64 {
	if factor <= 0 {
		return 0
	}
	return math.Log2(factor)
}

// aspectOption is a labelled value for video-aspect-override.
type aspectOption struct {
	label string
	value string
}

// aspectOptions returns the aspect-ratio menu entries. "Default" (-1) lets
// mpv use the container aspect.
func aspectOptions() []aspectOption {
	return []aspectOption{
		{"Default", "-1"},
		{"16:9", "1.7778"},
		{"4:3", "1.3333"},
		{"1.85:1", "1.85"},
		{"2.35:1", "2.35"},
	}
}

// zoomOptions returns the zoom factors offered in the menu.
func zoomOptions() []float64 {
	return []float64{0.5, 1, 1.5, 2}
}
