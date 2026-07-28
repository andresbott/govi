# Features — status and location

Check here before adding a capability: the gap may be deliberate or already
have a chosen direction. See [architecture.md](architecture.md) for layering,
[player.md](player.md) for player internals.

## Implemented

| Feature | Where |
|---|---|
| Play file from CLI (`govi <file>`) | `app/cmd/root.go`, `app/player/player.go` |
| Idle screen (launch with no file, after stop/EOF) | `app/player/idle.go`, mpv `idle-active` observed in `mpvlog.go` |
| Embedded placeholder image + config override | `app/player/idle.go` (`go:embed assets/idle.jpg` backdrop + `assets/logo.png` glyph, `loadPlaceholder`, `loadLogo`) |
| Drag-and-drop to load/replace video | `app/player/input.go` (`SetDropCallback`; first path wins, no type filtering) |
| YAML config + `GOVI_` env overrides + `--config` flag. Keys: `shortcuts:`, `placeholderImage:`, `volume:`, `muted:`. Two writers share the file (`saveShortcuts`, `saveAudioState`) and both preserve the other's keys via `updateConfig` | `app/cmd/config.go` (`<UserConfigDir>/govi/config.yaml`) |
| User-rebindable shortcuts (2 slots/action, `none` = empty slot, so `["none"]` unbinds and `["none","k"]` keeps a secondary in place) | `app/player/keymap.go`, registry in `actions.go` |
| Action registry (19 actions: play-pause, stop, seek ± 5 s, seek ± 10 %, volume ±, mute, progress (show/hide), info, help, fullscreen, next/previous-video, move-to-trash, delete-file, preferences, quit) | `app/player/actions.go` |
| Two seek granularities: ±5 s on `left`/`right` (`seek … relative`), ±10 % of the duration on `shift+left`/`shift+right` (`seek … relative-percent`, so mpv derives the offset and a live stream is a no-op rather than a bogus jump); both bring the control bar up instead of flashing text | `app/player/seek.go`, registry in `actions.go` |
| Preferences overlay: rebind shortcuts by clicking a slot and pressing a key, `×` clears a slot, `←` on an empty slot restores that action's defaults; persists to config.yaml (`shortcuts:` section rewritten, comments not preserved; Esc not bindable via UI, editable in YAML) | `app/player/overlay_prefs.go`, save in `app/cmd/config.go` (`saveShortcuts`) |
| Move-to-trash + permanent delete with Enter/Esc confirmation overlay (unbound by default, not in menu; user must bind keys) | `app/player/delete.go`, confirm handling in `input.go`, OS trash in `libs/trash` |
| Esc context sensitivity (close overlay → exit fullscreen → bound action) | `app/player/input.go` key callback |
| Info overlay grouped into File / Video / Audio / Subtitles sections (folder and file name split, duration, every track of each kind listed with `▶` on the selected one; mpv props, 1 s refresh, `—` for unavailable, `none` for a kind with no tracks) | model + grouping in `app/player/mediainfo.go`, mpv reads + layout in `overlay_info.go`, formatters in `format.go` |
| Help overlay (generated from registry + effective keymap): two side-by-side columns so the panel is half as tall, each listing the action with its `primary` and `secondary` binding in separate cells | `app/player/overlay_help.go` (`splitHelpRows`, `layoutHelpColumn`) |
| Right-click context menu with hover submenus, window clamping | `app/player/menu.go` |
| Audio/Video/Subtitle track selection from mpv `track-list` | `app/player/tracks.go`, `menu.go` (`trackSubmenu`) |
| Aspect-ratio + zoom submenus (`video-aspect-override`, log2 `video-zoom`) | `app/player/tracks.go` (`aspectOptions`, `zoomToVideoZoom`) |
| Idle-aware menu (only Help/Fullscreen/Quit when idle) | `app/player/menu.go` (`buildMenu`) |
| Fullscreen on most-occupied monitor, geometry restore | `app/player/player.go` (`toggleFullscreen`, `pickMonitor`) |
| Double-click the video to toggle fullscreen. A **single** click does nothing — click-to-pause was removed 2026-07-26; a click with the menu open just closes it | `app/player/input.go` (`primaryClickOutcome`) |
| Folder playlist: next/previous video among siblings of the opened file (PageDown/PageUp, menu entries; scanned once on open, missing files skipped and dropped, trash/delete auto-advances) | `app/player/playlist.go` (`scanPlaylist`, `advance`, `advanceAfterRemoval`) |
| Auto-advance: when a file ends it continues with the next playlist entry. Only `eof` advances — `stop` (our own loadfile / the stop action), `error` (unplayable file, which would otherwise race through the folder) and `quit` do not | `app/player/autoadvance.go` (`endsWithEOF`, `noteEndFile`, `handleEndOfFile`), armed in `mpvlog.go`, consumed in `loop` |
| Seeking off the end continues with the next video (both `→` and `shift+→`), checked before the command is sent because mpv clamps a seek to the last keyframe rather than overshooting the end — so a 10 % seek near the end would otherwise never trigger end-of-file. Falls through to mpv at the last entry, with no playlist, or on a stream with no known duration | `app/player/seek.go` (`passesEnd`, `percentDelta`, `advancePastEnd`) |
| Status flash (OSD): playlist position `2 / 5 name.mp4` on next/previous — its only user; bottom-center, 1.2 s, no timer (the loop already redraws every 50 ms). Every flash is a snapshot: progress and volume are both the control bar's job, so no flash re-reads itself. The `Volume 45%` / `Muted` flash was removed 2026-07-26 (with `volumeStatus`); the volume slider and mute glyph are the whole readout | `app/player/osd.go`, drawn from `ui.go` |
| Bottom control bar: play/pause button, a viewport-wide seek slider, a mute button whose glyph follows the mute state and level, and a 120 dp volume slider. Both sliders are the same widget down to the blue fill, so a drag behaves identically and only their width and place in the row tell them apart; auto-hides 1 s after the last pointer input with a quick 150 ms fade in/out, and stays up while *either* knob is dragged | `app/player/controls.go` (`updateControls`, `layoutSlider`), volume state in `volume.go`, drawn from `ui.go`, revealed from `input.go` and the seek/volume actions (`revealControls`) |
| Mouse pointer auto-hide in fullscreen: the pointer disappears 3 s after the mouse stops moving and comes straight back on the next move. Fullscreen only — in a window the pointer is how the user reaches the title bar and the desktop. Stays up while a slider knob is held or the context menu is open, and reappears on leaving fullscreen; `CursorHidden`, not `CursorDisabled`, so the pointer still moves freely and can leave for another screen | `app/player/cursor.go` (`cursorHidden`, `syncCursor`, `handlePointerMove`, `pointerBusy`), loop hook in `player.go`, cursor callback in `input.go` |
| Volume control by drag: the volume slider sets an absolute level (`set volume`, rounded to a whole percent) while it is dragged, and follows mpv's observed `volume` between drags, so a level changed by keyboard or menu shows on it. Clicking the volume button toggles mute. `volumeMax` is kept equal to mpv's `volume-max` option (`volumeMaxOption()`) so the slider's travel and mpv's ceiling cannot drift | `app/player/volume.go`, wired in `controls.go` (`updateControls`), observed in `mpvlog.go` |
| Volume and mute persist across runs: written to `volume:` / `muted:` in config.yaml, debounced 500 ms after the last change (a drag fires a set-volume per pointer move, so an immediate save would rewrite the file dozens of times per drag), plus a flush on shutdown so a change in the last half second is not lost. Restored as an mpv *option* before `Initialize`, so playback starts at the level instead of stepping up to it audibly. An absent `volume:` leaves mpv's own default alone — distinguished from a saved `0` by the `volumeUnset` sentinel, since a fresh install must start neither muted nor at full blast. Hand-edited out-of-range levels are clamped, not rejected | `app/player/volume.go` (`saveVolumeIfDue`, `flushVolumeSave`, `startVolume`, `applyAudioState`), loop hook + `mpvOptions` in `player.go`, `saveAudioState`/`updateConfig` in `app/cmd/config.go` |
| Progress is reported by the control bar, not by text: the seek shortcuts (`←`/`→`, `shift+←`/`shift+→`) bring the bar up (`revealControls`), and its knob re-reads the observed position every frame, so it tracks a seek mpv is still applying. Replaced the `1:10 / 3:20 35%` flash — no time overlay is drawn any more | `app/player/controls.go`, `seek.go` (`runSeek`) |
| The `o` action **toggles** the bar: it reveals it, and dismisses it again when it is already up (`hideControls`), fading out from whatever alpha it has rather than cutting. Since the bar is the progress readout, the key that summons it has to be the one that puts it away — otherwise a keyboard-only user waits out the 1 s auto-hide for an unobstructed picture. A bar already fading out counts as *not* showing (`controlsShowing`, distinct from `controlsVisible`), so the key brings it back instead of appearing to do nothing | `app/player/controls.go` (`hideControls`, `controlsShowing`), `toggleProgress` in `player.go` |
| Key auto-repeat for held keys, throttled per action (`action.repeat`): 5 s seek/volume ~50 ms, next/previous and the 10 % seek 300 ms (each fire loads a file, or covers a tenth of the video); press-only actions (quit, delete, play-pause…) ignore repeat | `app/player/actions.go` (`repeat` field), `app/player/input.go` (`dispatchKey`) |
| Neighbour prefetch: page-cache warm-up (head 2 MiB + tail 256 KiB, read-only) of the next/previous entry after every playlist change; each start cancels the previous generation | `app/player/prefetch.go` (`prefetcher`, `warmFile`, `neighbors`) |
| Structured logging with trace level; mpv logs forwarded as `component=mpv` | `internal/logging/`, `app/player/mpvlog.go` |
| `version` subcommand (linker-stamped) | `app/cmd/root.go`, `app/metainfo/` |
| Desktop integration: launcher entry, hicolor icons, video mime handler (`.deb` only) | `zarf/govi.desktop`, `zarf/govi.svg` + PNGs (rendered from it; artwork source `zarf/design/icon.svg`), `.goreleaser.yaml` nfpm contents; window class pinned in `app/player/player.go` (`appID`) |

## Not implemented — deliberately deferred (spec, "Out of scope")

Do **not** add these unprompted; the user chose to defer them:

| Feature | Note |
|---|---|
| Richer OSD (state icons) | the OSD went the other way: the seek bar replaced the progress flash outright, and on 2026-07-26 the volume bar arrived as a control-bar slider and took the `Volume 45%` / `Muted` flash with it. Playlist position is the only flash left. Playback-state icons were never asked for |
| Elapsed/total time text anywhere in the UI | removed 2026-07-26 with the progress flash — the bar is the whole readout. `humanClock` went with it (dead code); restore it from git if a time label is ever wanted |
| Equalizer-style A/V settings (brightness, delays…) | deferred in design spec |

## Not implemented — no decision recorded

Subtitle loading from file, playback speed, screenshots. Nothing in the spec
chooses a direction; ask before building.

## Quirks that are by design, not bugs

- `govi` with no args opens the idle-screen window (used to print help);
  `govi --help` still prints help.
- Unknown flags print help and exit 0 (`SetFlagErrorFunc` in `root.go`).
- Menu-only actions (currently `stop`) show the marker "menu" in the help
  overlay's primary-binding column ("unbound" for the ones the menu does not
  offer either, currently trash and delete).
