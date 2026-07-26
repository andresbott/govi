# Features — status and location

Check here before adding a capability: the gap may be deliberate or already
have a chosen direction. See [architecture.md](architecture.md) for layering,
[player.md](player.md) for player internals.

## Implemented

| Feature | Where |
|---|---|
| Play file from CLI (`govi <file>`) | `app/cmd/root.go`, `app/player/player.go` |
| Idle screen (launch with no file, after stop/EOF) | `app/player/idle.go`, mpv `idle-active` observed in `mpvlog.go` |
| Embedded placeholder image + config override | `app/player/idle.go` (`go:embed assets/idle.png`, `loadPlaceholder`) |
| Drag-and-drop to load/replace video | `app/player/input.go` (`SetDropCallback`; first path wins, no type filtering) |
| YAML config + `GOVI_` env overrides + `--config` flag | `app/cmd/config.go` (`<UserConfigDir>/govi/config.yaml`) |
| User-rebindable shortcuts (2 slots/action, `none` = empty slot, so `["none"]` unbinds and `["none","k"]` keeps a secondary in place) | `app/player/keymap.go`, registry in `actions.go` |
| Action registry (19 actions: play-pause, stop, seek ± 5 s, seek ± 10 %, volume ±, mute, progress, info, help, fullscreen, next/previous-video, move-to-trash, delete-file, preferences, quit) | `app/player/actions.go` |
| Two seek granularities: ±5 s on `left`/`right` (`seek … relative`), ±10 % of the duration on `shift+left`/`shift+right` (`seek … relative-percent`, so mpv derives the offset and a live stream is a no-op rather than a bogus jump); both bring the control bar up instead of flashing text | `app/player/seek.go`, registry in `actions.go` |
| Preferences overlay: rebind shortcuts by clicking a slot and pressing a key, `×` clears a slot, `←` on an empty slot restores that action's defaults; persists to config.yaml (`shortcuts:` section rewritten, comments not preserved; Esc not bindable via UI, editable in YAML) | `app/player/overlay_prefs.go`, save in `app/cmd/config.go` (`saveShortcuts`) |
| Move-to-trash + permanent delete with Enter/Esc confirmation overlay (unbound by default, not in menu; user must bind keys) | `app/player/delete.go`, confirm handling in `input.go`, OS trash in `libs/trash` |
| Esc context sensitivity (close overlay → exit fullscreen → bound action) | `app/player/input.go` key callback |
| Info overlay grouped into File / Video / Audio / Subtitles sections (folder and file name split, duration, every track of each kind listed with `▶` on the selected one; mpv props, 1 s refresh, `—` for unavailable, `none` for a kind with no tracks) | model + grouping in `app/player/mediainfo.go`, mpv reads + layout in `overlay_info.go`, formatters in `format.go` |
| Help overlay (generated from registry + effective keymap) | `app/player/overlay_help.go` |
| Right-click context menu with hover submenus, window clamping | `app/player/menu.go` |
| Audio/Video/Subtitle track selection from mpv `track-list` | `app/player/tracks.go`, `menu.go` (`trackSubmenu`) |
| Aspect-ratio + zoom submenus (`video-aspect-override`, log2 `video-zoom`) | `app/player/tracks.go` (`aspectOptions`, `zoomToVideoZoom`) |
| Idle-aware menu (only Help/Fullscreen/Quit when idle) | `app/player/menu.go` (`buildMenu`) |
| Fullscreen on most-occupied monitor, geometry restore | `app/player/player.go` (`toggleFullscreen`, `pickMonitor`) |
| Double-click the video to toggle fullscreen. A **single** click does nothing — click-to-pause was removed 2026-07-26; a click with the menu open just closes it | `app/player/input.go` (`primaryClickOutcome`) |
| Folder playlist: next/previous video among siblings of the opened file (PageDown/PageUp, menu entries; scanned once on open, missing files skipped and dropped, trash/delete auto-advances) | `app/player/playlist.go` (`scanPlaylist`, `advance`, `advanceAfterRemoval`) |
| Auto-advance: when a file ends it continues with the next playlist entry. Only `eof` advances — `stop` (our own loadfile / the stop action), `error` (unplayable file, which would otherwise race through the folder) and `quit` do not | `app/player/autoadvance.go` (`endsWithEOF`, `noteEndFile`, `handleEndOfFile`), armed in `mpvlog.go`, consumed in `loop` |
| Seeking off the end continues with the next video (both `→` and `shift+→`), checked before the command is sent because mpv clamps a seek to the last keyframe rather than overshooting the end — so a 10 % seek near the end would otherwise never trigger end-of-file. Falls through to mpv at the last entry, with no playlist, or on a stream with no known duration | `app/player/seek.go` (`passesEnd`, `percentDelta`, `advancePastEnd`) |
| Status flash (OSD): playlist position `2 / 5 name.mp4` on next/previous, `Volume 45%` / `Muted` on volume change and mute; bottom-center, 1.2 s, no timer (the loop already redraws every 50 ms). Every flash is a snapshot — progress is the control bar's job, so no flash re-reads itself | `app/player/osd.go`, drawn from `ui.go` |
| Bottom control bar: 4 dp track (played part bright red, remainder translucent white) with a draggable white knob that scrubs, spanning the viewport width between a working play/pause button and a placeholder volume button; auto-hides 2 s after the last pointer input with a quick 150 ms fade in/out, and stays up while the knob is dragged | `app/player/controls.go`, drawn from `ui.go`, revealed from `input.go` and the seek actions (`revealControls`) |
| Progress is reported by the control bar, not by text: the seek shortcuts (`←`/`→`, `shift+←`/`shift+→`) and the on-demand `o` action bring the bar up (`revealControls`), and its knob re-reads the observed position every frame, so it tracks a seek mpv is still applying. Replaced the `1:10 / 3:20 35%` flash — no time overlay is drawn any more | `app/player/controls.go`, `seek.go` (`runSeek`), `showProgress` in `player.go` |
| Key auto-repeat for held keys, throttled per action (`action.repeat`): 5 s seek/volume ~50 ms, next/previous and the 10 % seek 300 ms (each fire loads a file, or covers a tenth of the video); press-only actions (quit, delete, play-pause…) ignore repeat | `app/player/actions.go` (`repeat` field), `app/player/input.go` (`dispatchKey`) |
| Neighbour prefetch: page-cache warm-up (head 2 MiB + tail 256 KiB, read-only) of the next/previous entry after every playlist change; each start cancels the previous generation | `app/player/prefetch.go` (`prefetcher`, `warmFile`, `neighbors`) |
| Structured logging with trace level; mpv logs forwarded as `component=mpv` | `internal/logging/`, `app/player/mpvlog.go` |
| `version` subcommand (linker-stamped) | `app/cmd/root.go`, `app/metainfo/` |
| Desktop integration: launcher entry, hicolor icons, video mime handler (`.deb` only) | `zarf/govi.desktop`, `zarf/govi.svg` + PNGs, `.goreleaser.yaml` nfpm contents; window class pinned in `app/player/player.go` (`appID`) |

## Not implemented — deliberately deferred (spec, "Out of scope")

Do **not** add these unprompted; the user chose to defer them:

| Feature | Note |
|---|---|
| Richer OSD (volume *bar*, state icons) | the seek bar was since asked for and built (`controls.go`), and it replaced the progress flash outright; a volume bar and playback-state icons still were not asked for |
| Elapsed/total time text anywhere in the UI | removed 2026-07-26 with the progress flash — the bar is the whole readout. `humanClock` went with it (dead code); restore it from git if a time label is ever wanted |
| Volume button action | the button is drawn in the control bar as a placeholder only; volume is keyboard/menu driven. Wiring it (a slider, or mute on click) was not asked for |
| Equalizer-style A/V settings (brightness, delays…) | deferred in design spec |

## Not implemented — no decision recorded

Subtitle loading from file, playback speed, screenshots. Nothing in the spec
chooses a direction; ask before building.

## Quirks that are by design, not bugs

- `govi` with no args opens the idle-screen window (used to print help);
  `govi --help` still prints help.
- Unknown flags print help and exit 0 (`SetFlagErrorFunc` in `root.go`).
- Menu-only actions (currently `stop`) show the marker "menu" in the help
  overlay.
