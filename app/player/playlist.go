package player

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// videoExts is the set of extensions scanPlaylist recognizes as videos when
// collecting siblings. The opened file itself is always included, whatever
// its extension — mpv, not the extension, decides if it is playable.
var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".webm": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".m4v": true, ".mpg": true, ".mpeg": true,
	".ts": true, ".m2ts": true, ".ogv": true, ".3gp": true, ".vob": true,
}

// playlist is the in-memory list of sibling videos built once when a file is
// opened, so next/previous never re-asks the OS for the folder contents.
type playlist struct {
	entries []string // absolute paths, sorted by base name
	idx     int      // position of the currently playing entry
}

// scanPlaylist lists the videos in the opened file's folder and returns a
// playlist positioned at that file. It returns nil when path is not a local
// file (URL, directory, unreadable folder), leaving the player playlist-less.
func scanPlaylist(path string) *playlist {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	dirents, err := os.ReadDir(filepath.Dir(abs))
	if err != nil {
		return nil
	}
	pl := &playlist{}
	for _, de := range dirents {
		if de.IsDir() {
			continue
		}
		full := filepath.Join(filepath.Dir(abs), de.Name())
		if full == abs || videoExts[strings.ToLower(filepath.Ext(de.Name()))] {
			pl.entries = append(pl.entries, full)
		}
	}
	sort.Strings(pl.entries)
	for i, e := range pl.entries {
		if e == abs {
			pl.idx = i
			break
		}
	}
	return pl
}

// firstVideo returns the alphabetically-first video in dir, or "" when dir has
// none (or cannot be read). Used to open a folder — `govi <dir>` or `govi .` —
// as if its first video had been passed directly: the file it returns is
// exactly the entry scanPlaylist then lands on.
func firstVideo(dir string) string {
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var vids []string
	for _, de := range dirents {
		if de.IsDir() {
			continue
		}
		if videoExts[strings.ToLower(filepath.Ext(de.Name()))] {
			vids = append(vids, filepath.Join(dir, de.Name()))
		}
	}
	if len(vids) == 0 {
		return ""
	}
	sort.Strings(vids)
	return vids[0]
}

// resolveStartPath turns the requested startup path into the file to open. A
// directory (`govi .` or `govi <dir>`) resolves to its first video so the whole
// folder plays as if that file had been opened. A regular file or a URL passes
// through unchanged. An empty path (bare `govi`) stays empty, and a folder with
// no videos returns "", so the caller falls back to the idle screen.
func resolveStartPath(path string) string {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return firstVideo(path)
	}
	return path
}

// current returns the entry the playlist points at, or "" when empty.
func (pl *playlist) current() string {
	if pl == nil || pl.idx < 0 || pl.idx >= len(pl.entries) {
		return ""
	}
	return pl.entries[pl.idx]
}

// advance moves by dir (+1 next, -1 previous) to the nearest entry for which
// exists returns true, dropping entries that no longer exist along the way.
// It returns the new current path, or "" (position unchanged) when no
// existing entry lies in that direction.
func (pl *playlist) advance(dir int, exists func(string) bool) string {
	for i := pl.idx + dir; i >= 0 && i < len(pl.entries); i += dir {
		if exists(pl.entries[i]) {
			pl.idx = i
			return pl.entries[i]
		}
		// Drop the vanished entry; when moving forward the removal shifts
		// the next candidate into i, so step back to revisit that slot.
		pl.entries = append(pl.entries[:i], pl.entries[i+1:]...)
		if dir > 0 {
			i -= dir
		}
	}
	return ""
}

// nextVideo plays the next sibling video in the playlist, if any.
func (p *Player) nextVideo() { p.playAdjacent(1) }

// prevVideo plays the previous sibling video in the playlist, if any.
func (p *Player) prevVideo() { p.playAdjacent(-1) }

// playAdjacent advances the playlist by dir and loads the resulting file,
// reporting whether it moved. Entries deleted behind the player's back are
// skipped and dropped.
func (p *Player) playAdjacent(dir int) bool {
	if p.pl == nil {
		return false
	}
	next := p.pl.advance(dir, fileExists)
	if next == "" {
		return false
	}
	p.loadFile(next)
	p.flashPosition()
	p.pf.start(p.pl.neighbors())
	return true
}

// fileExists is playlist.advance's liveness check for real files.
func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// advanceAfterRemoval drops a trashed/deleted file from the playlist and,
// image-viewer style, plays whatever the playlist now points at (the next
// video, or the previous one when the last entry was removed). With no
// playlist or no entries left, playback stays on the idle screen.
func (p *Player) advanceAfterRemoval(path string) {
	if p.pl == nil {
		return
	}
	p.pl.remove(path)
	if cur := p.pl.current(); cur != "" {
		p.loadFile(cur)
		p.pf.start(p.pl.neighbors())
	}
}

// remove drops path from the playlist (e.g. after trash/delete). If the
// removed entry was current, the playlist points at the following entry, or
// the previous one when the last entry was removed.
func (pl *playlist) remove(path string) {
	for i, e := range pl.entries {
		if e != path {
			continue
		}
		pl.entries = append(pl.entries[:i], pl.entries[i+1:]...)
		if i < pl.idx || pl.idx >= len(pl.entries) {
			pl.idx--
		}
		return
	}
}
