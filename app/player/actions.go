package player

// actionID names a user-triggerable action. IDs double as the config keys
// under `shortcuts:` and as the stable identity shared by the keymap, the
// help overlay, and the context menu.
type actionID string

const (
	actPlayPause   actionID = "play-pause"
	actStop        actionID = "stop"
	actSeekForward actionID = "seek-forward"
	actSeekBack    actionID = "seek-back"
	actVolumeUp    actionID = "volume-up"
	actVolumeDown  actionID = "volume-down"
	actMute        actionID = "mute"
	actInfo        actionID = "info"
	actHelp        actionID = "help"
	actPreferences actionID = "preferences"
	actFullscreen  actionID = "fullscreen"
	actNextVideo   actionID = "next-video"
	actPrevVideo   actionID = "previous-video"
	actTrash       actionID = "move-to-trash"
	actDelete      actionID = "delete-file"
	actQuit        actionID = "quit"
)

// action is one entry in the registry. defaults holds the built-in key
// strings (index 0 main, index 1 secondary); an empty defaults slice means
// the action has no key until the user binds one. inMenu marks actions the
// context menu offers, so the help overlay can point keyless actions at the
// menu or mark them unbound.
type action struct {
	id       actionID
	label    string
	defaults []string
	fn       func(p *Player)
	inMenu   bool
}

// defaultActions returns the registry in display order (used by the help
// overlay and menu). Each fn drives mpv or window state directly.
func defaultActions() []action {
	return []action{
		{actPlayPause, "Play / Pause", []string{"space", "k"}, (*Player).togglePause, true},
		{actStop, "Stop", nil, (*Player).stop, true},
		{actSeekForward, "Seek Forward", []string{"right"}, func(p *Player) { p.seek(5) }, false},
		{actSeekBack, "Seek Back", []string{"left"}, func(p *Player) { p.seek(-5) }, false},
		{actVolumeUp, "Volume Up", []string{"up", "+"}, func(p *Player) { p.changeVolume(5) }, true},
		{actVolumeDown, "Volume Down", []string{"down", "-"}, func(p *Player) { p.changeVolume(-5) }, true},
		{actMute, "Mute", []string{"m"}, (*Player).toggleMute, true},
		{actInfo, "Video Info", []string{"i"}, func(p *Player) { p.toggleOverlay(overlayInfo) }, true},
		{actHelp, "Shortcut Help", []string{"?", "h"}, func(p *Player) { p.toggleOverlay(overlayHelp) }, true},
		{actPreferences, "Preferences", nil, (*Player).togglePrefs, true},
		{actFullscreen, "Fullscreen", []string{"f", "f11"}, (*Player).toggleFullscreen, true},
		{actNextVideo, "Next Video", []string{"pagedown"}, (*Player).nextVideo, true},
		{actPrevVideo, "Previous Video", []string{"pageup"}, (*Player).prevVideo, true},
		{actTrash, "Move to Trash", nil, (*Player).moveToTrash, false},
		{actDelete, "Delete File", nil, (*Player).askDeleteFile, false},
		{actQuit, "Quit", []string{"q", "esc"}, func(p *Player) { p.window.SetShouldClose(true) }, true},
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
