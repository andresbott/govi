package player

import (
	"fmt"
	"time"
)

// seekPercentStep is how far the coarse seek moves per fire, as a share of the
// file's total duration.
const seekPercentStep = 10

// seekCommand builds the mpv command for a fine seek of delta seconds
// (negative = backwards), relative to the current position.
func seekCommand(delta int) []string {
	return []string{"seek", fmt.Sprintf("%d", delta), "relative"}
}

// seekPercentCommand builds the mpv command for a coarse seek of pct percent of
// the file's duration (negative = backwards). mpv works the offset out itself
// from the duration it already knows, so the player never reads `duration` —
// which is exactly what makes this a no-op on a live stream rather than a jump
// to a bogus position.
func seekPercentCommand(pct int) []string {
	return []string{"seek", fmt.Sprintf("%d", pct), "relative-percent"}
}

// passesEnd reports whether seeking step seconds from pos lands at or past the
// end of a file of dur seconds. A non-positive dur (live stream, or a duration
// mpv does not know yet) has no end to run off, and only forward seeks can.
func passesEnd(pos, dur, step float64) bool {
	return dur > 0 && step > 0 && pos+step >= dur
}

// percentDelta converts a percentage of dur into seconds, mirroring what mpv
// works out itself for a relative-percent seek. Zero for an unknown duration,
// where the percentage means nothing.
func percentDelta(dur float64, pct int) float64 {
	if dur <= 0 {
		return 0
	}
	return dur * float64(pct) / 100
}

// hasNext reports whether an entry follows the current one.
func (pl *playlist) hasNext() bool {
	return pl != nil && pl.idx+1 < len(pl.entries)
}

// advancePastEnd continues with the next playlist entry when a seek of step
// seconds from pos would run off the end of a file of dur seconds, reporting
// whether it did. It exists because mpv *clamps* a seek to the last keyframe
// before the end rather than overshooting it: a 10 % seek near the end lands
// inside the file and never reports end-of-file, so without this the seek would
// silently do nothing useful instead of continuing with the next video.
//
// It deliberately declines when nothing follows (last entry, no playlist): the
// seek then goes to mpv, whose own end-of-file handling (idle screen) is right.
func (p *Player) advancePastEnd(pos, dur, step float64) bool {
	if !passesEnd(pos, dur, step) || !p.pl.hasNext() {
		return false
	}
	p.log.Debug("seek past end of file, advancing playlist", "pos", pos, "duration", dur, "step", step)
	return p.playAdjacent(1)
}

// seek moves playback position by delta seconds (negative = backwards),
// relative to the current position, and brings the control bar up to show where
// it landed. A forward seek that runs off the end continues with the next video
// instead. mpv ignores it when no file is loaded.
func (p *Player) seek(delta int) {
	if p.advancePastEnd(p.playbackPos(), p.playbackDur(), float64(delta)) {
		return
	}
	p.runSeek(seekCommand(delta), "delta", delta)
}

// seekPercent moves playback position by pct percent of the file's duration
// (negative = backwards) and brings the control bar up. Like seek, running off
// the end continues with the next video.
func (p *Player) seekPercent(pct int) {
	dur := p.playbackDur()
	if p.advancePastEnd(p.playbackPos(), dur, percentDelta(dur, pct)) {
		return
	}
	p.runSeek(seekPercentCommand(pct), "percent", pct)
}

// runSeek sends a seek command and brings the control bar up so the new position
// is visible. amountKey and amount name the step in the error log, which is the
// only thing that differs between the two seek flavours.
//
// The bar replaces the text flash these used to raise: its knob is re-read from
// the observed position on every frame (syncProgressKnob), so it tracks a seek
// mpv is still applying without the flash's re-read machinery.
func (p *Player) runSeek(cmd []string, amountKey string, amount int) {
	if err := p.mpv.Command(cmd); err != nil {
		p.log.Error("mpv seek", amountKey, amount, "err", err)
		return
	}
	p.revealControls(time.Now())
}
