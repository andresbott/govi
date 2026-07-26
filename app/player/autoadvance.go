package player

import mpv "github.com/gen2brain/go-mpv"

// endsWithEOF reports whether an mpv end-file reason means the file ran out —
// which is what auto-advance reacts to. mpv reports the same `eof` reason
// whether playback reached the end on its own or a seek jumped past it, so
// keyboard navigation off the end of a file advances just like watching it
// through does.
//
// Every other reason must not advance: `stop` is what our own loadfile/stop
// produce (manual next/previous already picked the entry — advancing again
// would skip one), `quit` fires during shutdown, `error` means the file was
// unplayable (advancing on it would race through the whole folder as fast as
// mpv can fail), and `redirect` is a playlist file being expanded, not an end.
func endsWithEOF(reason mpv.Reason) bool {
	return reason == mpv.EndFileEOF
}

// noteEndFile records an end-file event from mpv's event goroutine. It only
// flags the pending advance (threading invariant 2/5: no mpv or player state
// from another thread); the main loop performs the load in handleEndOfFile.
func (p *Player) noteEndFile(reason mpv.Reason) {
	if !endsWithEOF(reason) {
		return
	}
	p.eofPending.Store(true)
}

// noteFileStart clears a pending advance because a new file has begun playing,
// also called from the event goroutine. mpv queues the end of the outgoing file
// and the start of the incoming one together, so this drops the flag in the one
// ordering loadFile cannot: the eof arriving after loadFile already ran.
func (p *Player) noteFileStart() {
	p.eofPending.Store(false)
}

// handleEndOfFile plays the next playlist entry once the current file ended by
// itself, called once per loop iteration on the main thread. The flag is
// consumed whether or not an entry follows: at the end of the folder mpv's own
// idle screen is the right result, and a stale flag would advance on the next
// file the user opens.
func (p *Player) handleEndOfFile() {
	if !p.eofPending.CompareAndSwap(true, false) {
		return
	}
	p.log.Debug("file ended, advancing playlist")
	p.playAdjacent(1)
}
