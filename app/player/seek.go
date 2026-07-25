package player

import "fmt"

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

// seek moves playback position by delta seconds (negative = backwards),
// relative to the current position, and flashes the resulting progress. mpv
// ignores it when no file is loaded.
func (p *Player) seek(delta int) {
	p.runSeek(seekCommand(delta), "delta", delta)
}

// seekPercent moves playback position by pct percent of the file's duration
// (negative = backwards) and flashes the resulting progress.
func (p *Player) seekPercent(pct int) {
	p.runSeek(seekPercentCommand(pct), "percent", pct)
}

// runSeek sends a seek command and flashes the resulting progress. amountKey and
// amount name the step in the error log, which is the only thing that differs
// between the two seek flavours.
func (p *Player) runSeek(cmd []string, amountKey string, amount int) {
	if err := p.mpv.Command(cmd); err != nil {
		p.log.Error("mpv seek", amountKey, amount, "err", err)
		return
	}
	p.flashProgress()
}
