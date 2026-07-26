package player

import (
	"fmt"
	"math"

	mpv "github.com/gen2brain/go-mpv"
)

// notePlaybackProp records an observed playback property.
func (p *Player) notePlaybackProp(name string, data any) {
	// mpv sends the change with no value when a property becomes unavailable
	// (time-pos and duration both do on stop). Zero is what the formatters
	// already treat as "unknown", so fall back to it rather than keeping the
	// value the previous file left behind.
	v, ok := data.(float64)
	if !ok {
		v = 0
	}
	switch name {
	case "time-pos":
		p.obsPos.Store(math.Float64bits(v))
	case "duration":
		p.obsDur.Store(math.Float64bits(v))
	}
}

// playbackPos is the last observed playback position in seconds.
func (p *Player) playbackPos() float64 {
	return math.Float64frombits(p.obsPos.Load())
}

// playbackDur is the last observed file duration in seconds.
func (p *Player) playbackDur() float64 {
	return math.Float64frombits(p.obsDur.Load())
}

// observePlayback asks mpv to report position and duration changes, so the
// render loop can read them from the cache instead of asking mpv itself. The
// userdata values tag these observations in the event pump.
func (p *Player) observePlayback() error {
	for id, name := range map[uint64]string{2: "time-pos", 3: "duration"} {
		if err := p.mpv.ObserveProperty(id, name, mpv.FormatDouble); err != nil {
			return fmt.Errorf("observe %s: %w", name, err)
		}
	}
	return nil
}
