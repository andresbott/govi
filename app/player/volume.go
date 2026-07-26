package player

import (
	"fmt"
	"math"
	"time"

	"gioui.org/widget"
	mpv "github.com/gen2brain/go-mpv"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// volumeMax is the loudest level the UI offers, and must stay equal to the
// volume-max option initMpv sets: the slider maps its full travel onto 0..this,
// so a larger mpv maximum would leave part of the range unreachable and a
// smaller one would let the knob ask for a level mpv silently clamps.
const volumeMax = 100

// volumeIconLoud is the level from which the icon shows the loud glyph. Purely
// cosmetic — it splits "quiet" from "loud" at the middle of the range.
const volumeIconLoud = volumeMax / 2

// volumeMaxOption is volumeMax as the string initMpv passes to mpv's volume-max
// option. Derived rather than written twice: the slider's travel and mpv's
// ceiling have to be the same number (see volumeMax).
func volumeMaxOption() string {
	return fmt.Sprintf("%d", volumeMax)
}

// The volume glyphs: like the play/pause pair these are vector paths, since the
// Go fonts carry no media glyphs at all (see controls.go).
var (
	iconVolumeUp   = mustIcon(icons.AVVolumeUp)
	iconVolumeDown = mustIcon(icons.AVVolumeDown)
	iconVolumeOff  = mustIcon(icons.AVVolumeOff)
)

// volumeFraction is vol as a share of volumeMax, in [0,1] — the knob position
// for a level.
func volumeFraction(vol float64) float32 {
	f := vol / volumeMax
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return float32(f)
}

// volumeTarget converts a knob position in [0,1] into an mpv level, clamped into
// the range the slider can display. Rounded to a whole percent: that is the unit
// the volume shortcuts step in and the saved level is stored in, so a drag landing
// on 44.999 (which is what float32(0.45) scales to) must not persist as a value the
// keyboard can never reproduce.
func volumeTarget(frac float32) float64 {
	v := math.Round(float64(frac) * volumeMax)
	if v < 0 {
		return 0
	}
	if v > volumeMax {
		return volumeMax
	}
	return v
}

// setVolumeCommand builds the mpv command for an absolute level: the slider says
// where the volume should be, unlike the volume shortcuts, which nudge it by a
// delta ("add volume"). Formatted as a whole percent, matching what volumeTarget
// already rounded to.
func setVolumeCommand(vol float64) []string {
	return []string{"set", "volume", fmt.Sprintf("%.0f", vol)}
}

// volumeIcon is the glyph for the volume button: crossed out while muted (the
// state, not what the click will do — the same convention as every other
// player), otherwise a quiet or loud speaker depending on the level.
func volumeIcon(vol float64, muted bool) *widget.Icon {
	switch {
	case muted:
		return iconVolumeOff
	case vol < volumeIconLoud:
		return iconVolumeDown
	default:
		return iconVolumeUp
	}
}

// volumeSaveDelay is how long the level has to sit still before it is written to
// disk. A drag fires a set-volume per pointer move, so saving on every change
// would rewrite the config dozens of times per drag; the loop already ticks every
// idleFrame, so this needs no timer of its own — the same reasoning as the status
// flash and the control bar's auto-hide.
const volumeSaveDelay = 500 * time.Millisecond

// startVolume is the mpv volume option for a configured level, and whether there
// is one to apply. An unset level leaves mpv's own default alone: forcing a value
// (100, say) would make a fresh install start at full blast. A hand-edited config
// can hold anything, so the level is clamped rather than rejected — a typo must
// not leave the knob pinned off the end of its track.
func startVolume(vol *int) (string, bool) {
	if vol == nil {
		return "", false
	}
	return fmt.Sprintf("%.0f", volumeTarget(volumeFraction(float64(*vol)))), true
}

// applyAudioState restores the configured mute flag, on the Player and in mpv:
// a restored flag with audible sound (or the reverse) would be the worst of both.
func (p *Player) applyAudioState(muted bool) {
	p.muted = muted
	if p.mpv == nil { // unit tests build a Player without mpv
		return
	}
	if err := p.mpv.SetProperty("mute", mpv.FormatFlag, muted); err != nil {
		p.log.Error("restore mute property", "muted", muted, "err", err)
	}
}

// noteVolumeChange arms the debounced save. Called from every path that changes
// the level or the mute flag; the loop writes it out once it settles.
func (p *Player) noteVolumeChange() {
	p.volumeSavePending = time.Now()
}

// noteVolumeChanged is what the keyboard and menu volume paths run after mpv has
// applied their change: it re-reads the resulting level, brings the control bar up
// so the slider and the mute glyph report it, and arms the save.
//
// It deliberately shows no text. The level used to flash as "Volume 45%" /
// "Muted"; that went away on 2026-07-26 in favour of the bar alone, the same trade
// the seek bar made when it replaced the progress flash. Don't reintroduce it.
//
// The mpv read is synchronous because this runs off a key press, not per frame
// (invariant 6), and it takes the level *back from mpv* rather than assuming the
// requested one, so a value clamped by volume-max is what the slider shows. The
// result is published to the observed cache so the knob does not sit stale until
// mpv's own notification arrives a frame or more later.
func (p *Player) noteVolumeChanged() {
	if p.mpv != nil { // unit tests build a Player without mpv
		p.noteVolumeProp(float64(p.propInt("volume")))
	}
	p.revealControls(time.Now())
	p.noteVolumeChange()
}

// volumeSaveDue reports whether an armed save has settled at now. A zero pending
// means nothing changed since the last write.
func volumeSaveDue(now, pending time.Time) bool {
	if pending.IsZero() {
		return false
	}
	return now.Sub(pending) >= volumeSaveDelay
}

// saveVolumeIfDue persists the level and mute flag once the last change has
// settled. Called from the render loop.
func (p *Player) saveVolumeIfDue(now time.Time) {
	if volumeSaveDue(now, p.volumeSavePending) {
		p.saveVolumeNow()
	}
}

// flushVolumeSave writes a pending level out regardless of the debounce, for the
// shutdown path: a change made in the last volumeSaveDelay before quitting would
// otherwise be lost, since nothing waits out the delay on the way down. A player
// with nothing pending writes nothing, so merely opening and closing the player
// does not rewrite the config.
func (p *Player) flushVolumeSave() {
	if !p.volumeSavePending.IsZero() {
		p.saveVolumeNow()
	}
}

// saveVolumeNow writes the observed level and the mute flag through the injected
// callback and disarms.
//
// It disarms before running the callback and on every outcome, including failure:
// an armed save that survived its own attempt would retry on every frame, which
// for a failing write means flooding the log. A missing callback (unit tests, and
// any future embedding) disarms too, for the same reason.
func (p *Player) saveVolumeNow() {
	p.volumeSavePending = time.Time{}
	if p.saveAudioState == nil {
		return
	}
	vol := int(math.Round(p.volumeLevel()))
	if err := p.saveAudioState(vol, p.muted); err != nil {
		// Logged and dropped, like every other runtime error: losing the saved
		// level must not take the player down.
		p.log.Error("save volume", "volume", vol, "muted", p.muted, "err", err)
	}
}

// noteVolumeProp records the observed volume. Like the playback properties, mpv
// sends the change with no value when the property becomes unavailable, and zero
// is what the knob and the icon already read as silence.
func (p *Player) noteVolumeProp(data any) {
	v, ok := data.(float64)
	if !ok {
		v = 0
	}
	p.obsVol.Store(math.Float64bits(v))
}

// volumeLevel is the last observed volume in percent.
func (p *Player) volumeLevel() float64 {
	return math.Float64frombits(p.obsVol.Load())
}

// observeVolume asks mpv to report volume changes, so the control bar reads the
// level from the cache instead of asking mpv on every frame it is visible (see
// threading invariant 6 in docs/agents/player.md). The userdata value tags the
// observation in the event pump.
func (p *Player) observeVolume() error {
	if err := p.mpv.ObserveProperty(4, "volume", mpv.FormatDouble); err != nil {
		return fmt.Errorf("observe volume: %w", err)
	}
	return nil
}

// syncVolumeKnob moves the volume knob to the current level. Reads the observed
// value, not mpv: this runs on every frame the bar is visible.
func (p *Player) syncVolumeKnob() {
	p.volume.Value = volumeFraction(p.volumeLevel())
}

// setVolume applies the level the volume knob was dragged to. Called while the
// drag is in progress, so the volume follows the knob rather than waiting for the
// release.
//
// The new level is published to the observed cache right away: mpv's own
// notification arrives a frame or more later, and until then the icon would still
// show the level the drag started from. It arms the save itself rather than going
// through noteVolumeChanged, since it already knows the level it just set and has
// no need to read it back or reveal a bar the pointer is holding open anyway.
func (p *Player) setVolume(frac float32) {
	if p.mpv == nil { // unit tests build a Player without mpv
		return
	}
	vol := volumeTarget(frac)
	if err := p.mpv.Command(setVolumeCommand(vol)); err != nil {
		p.log.Error("mpv set volume", "fraction", frac, "err", err)
		return
	}
	p.noteVolumeProp(vol)
	p.noteVolumeChange()
}
