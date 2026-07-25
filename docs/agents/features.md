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
| Action registry (16 actions: play-pause, stop, seek ±, volume ±, mute, info, help, fullscreen, next/previous-video, move-to-trash, delete-file, preferences, quit) | `app/player/actions.go` |
| Preferences overlay: rebind shortcuts by clicking a slot and pressing a key, `×` clears a slot, `←` on an empty slot restores that action's defaults; persists to config.yaml (`shortcuts:` section rewritten, comments not preserved; Esc not bindable via UI, editable in YAML) | `app/player/overlay_prefs.go`, save in `app/cmd/config.go` (`saveShortcuts`) |
| Move-to-trash + permanent delete with Enter/Esc confirmation overlay (unbound by default, not in menu; user must bind keys) | `app/player/delete.go`, confirm handling in `input.go` |
| Esc context sensitivity (close overlay → exit fullscreen → bound action) | `app/player/input.go` key callback |
| Info overlay (mpv props, 1 s refresh, `—` for unavailable) | `app/player/overlay_info.go`, formatters in `format.go` |
| Help overlay (generated from registry + effective keymap) | `app/player/overlay_help.go` |
| Right-click context menu with hover submenus, window clamping | `app/player/menu.go` |
| Audio/Video/Subtitle track selection from mpv `track-list` | `app/player/tracks.go`, `menu.go` (`trackSubmenu`) |
| Aspect-ratio + zoom submenus (`video-aspect-override`, log2 `video-zoom`) | `app/player/tracks.go` (`aspectOptions`, `zoomToVideoZoom`) |
| Idle-aware menu (only Help/Fullscreen/Quit when idle) | `app/player/menu.go` (`buildMenu`) |
| Fullscreen on most-occupied monitor, geometry restore | `app/player/player.go` (`toggleFullscreen`, `pickMonitor`) |
| Click video to pause (double-click fullscreen); suppressed while menu open | `app/player/input.go` |
| Folder playlist: next/previous video among siblings of the opened file (PageDown/PageUp, menu entries; scanned once on open, missing files skipped and dropped, trash/delete auto-advances) | `app/player/playlist.go` (`scanPlaylist`, `advance`, `advanceAfterRemoval`) |
| Structured logging with trace level; mpv logs forwarded as `component=mpv` | `internal/logging/`, `app/player/mpvlog.go` |
| `version` subcommand (linker-stamped) | `app/cmd/root.go`, `app/metainfo/` |

## Not implemented — deliberately deferred (spec, "Out of scope")

Do **not** add these unprompted; the user chose to defer them:

| Feature | Note |
|---|---|
| OSD flash feedback (volume bar, state icons) | user will handle OSD later |
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
- README's "Status: only the CLI skeleton" section is stale (predates the
  player); the Development/Release sections are current.
