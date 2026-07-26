package player

import "time"

// actionID names a user-triggerable action. IDs double as the config keys
// under `shortcuts:` and as the stable identity shared by the keymap, the
// help overlay, and the context menu.
type actionID string

const (
	actPlayPause      actionID = "play-pause"
	actStop           actionID = "stop"
	actSeekForward    actionID = "seek-forward"
	actSeekBack       actionID = "seek-back"
	actSeekForwardPct actionID = "seek-forward-percent"
	actSeekBackPct    actionID = "seek-back-percent"
	actVolumeUp       actionID = "volume-up"
	actVolumeDown     actionID = "volume-down"
	actMute           actionID = "mute"
	actProgress       actionID = "progress"
	actInfo           actionID = "info"
	actHelp           actionID = "help"
	actPreferences    actionID = "preferences"
	actFullscreen     actionID = "fullscreen"
	actNextVideo      actionID = "next-video"
	actPrevVideo      actionID = "previous-video"
	actTrash          actionID = "move-to-trash"
	actDelete         actionID = "delete-file"
	actQuit           actionID = "quit"
)

// action is one entry in the registry. defaults holds the built-in key
// strings (index 0 main, index 1 secondary); an empty defaults slice means
// the action has no key until the user binds one. inMenu marks actions the
// context menu offers, so the help overlay can point keyless actions at the
// menu or mark them unbound.
//
// repeat is the minimum interval between two fires while the key is held down;
// zero means the action ignores auto-repeat and only reacts to a fresh press
// (the default — repeating quit or delete would be a disaster). An interval is
// needed rather than a bool because the keyboard repeat rate is an OS setting:
// at ~30 repeats per second an unthrottled next-video would race through a
// whole folder before the user lets go.
//
// The registry below uses positional literals: id, label, defaults, fn,
// inMenu, repeat.
type action struct {
	id       actionID
	label    string
	defaults []string
	fn       func(p *Player)
	inMenu   bool
	repeat   time.Duration
}

// Repeat intervals for held-down keys. repeatContinuous is effectively "as
// fast as the OS repeats" for cheap, small-step actions (5 s seek, volume
// nudge); repeatNavigate is deliberately slower for actions that move a long
// way per fire — next/previous load a whole new file, and a held percentage
// seek would otherwise run off the end of the video before the key is released.
const (
	repeatContinuous = 50 * time.Millisecond
	repeatNavigate   = 300 * time.Millisecond
)

// defaultActions returns the registry in display order (used by the help
// overlay and menu). Each fn drives mpv or window state directly.
func defaultActions() []action {
	return []action{
		{actPlayPause, "Play / Pause", []string{"space", "k"}, (*Player).togglePause, true, 0},
		{actStop, "Stop", nil, (*Player).stop, true, 0},
		{actSeekForward, "Seek Forward", []string{"right"}, func(p *Player) { p.seek(5) }, false, repeatContinuous},
		{actSeekBack, "Seek Back", []string{"left"}, func(p *Player) { p.seek(-5) }, false, repeatContinuous},
		{actSeekForwardPct, "Seek Forward 10%", []string{"shift+right"}, func(p *Player) { p.seekPercent(seekPercentStep) }, false, repeatNavigate},
		{actSeekBackPct, "Seek Back 10%", []string{"shift+left"}, func(p *Player) { p.seekPercent(-seekPercentStep) }, false, repeatNavigate},
		{actVolumeUp, "Volume Up", []string{"up", "+"}, func(p *Player) { p.changeVolume(5) }, true, repeatContinuous},
		{actVolumeDown, "Volume Down", []string{"down", "-"}, func(p *Player) { p.changeVolume(-5) }, true, repeatContinuous},
		{actMute, "Mute", []string{"m"}, (*Player).toggleMute, true, 0},
		{actProgress, "Show / Hide Progress", []string{"o"}, (*Player).toggleProgress, true, 0},
		{actInfo, "Video Info", []string{"i"}, func(p *Player) { p.toggleOverlay(overlayInfo) }, true, 0},
		{actHelp, "Shortcut Help", []string{"?", "h"}, func(p *Player) { p.toggleOverlay(overlayHelp) }, true, 0},
		{actPreferences, "Preferences", nil, (*Player).togglePrefs, true, 0},
		{actFullscreen, "Fullscreen", []string{"f", "f11"}, (*Player).toggleFullscreen, true, 0},
		{actNextVideo, "Next Video", []string{"pagedown"}, (*Player).nextVideo, true, repeatNavigate},
		{actPrevVideo, "Previous Video", []string{"pageup"}, (*Player).prevVideo, true, repeatNavigate},
		{actTrash, "Move to Trash", nil, (*Player).moveToTrash, false, 0},
		{actDelete, "Delete File", nil, (*Player).askDeleteFile, false, 0},
		{actQuit, "Quit", []string{"q", "esc"}, func(p *Player) { p.window.SetShouldClose(true) }, true, 0},
	}
}

// actionByID indexes the registry for dispatch.
func actionByID() map[actionID]action {
	m := make(map[actionID]action)
	for _, a := range defaultActions() {
		m[a.id] = a
	}
	return m
}

// actionDefaults returns the built-in key strings per action, used to fill
// in any action the user's config did not mention.
func actionDefaults() map[actionID][]string {
	m := make(map[actionID][]string)
	for _, a := range defaultActions() {
		if len(a.defaults) > 0 {
			m[a.id] = a.defaults
		}
	}
	return m
}
