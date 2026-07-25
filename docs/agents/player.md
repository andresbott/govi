# Player subsystem — GLFW + libmpv + Gio in one framebuffer

`app/player` is the whole UI. This file records the invariants that will
break playback (or colors, or shutdown) if moved. Entry: `player.Run(ctx,
path, cfg)` — empty path starts on the idle screen. See
[architecture.md](architecture.md) for the registry design.

## Threading invariants (violations crash or deadlock)

1. **Main OS thread owns GLFW and GL.** `init()` calls
   `runtime.LockOSThread()`; every GLFW/GL/Gio/mpv-render call happens on the
   main loop. Never call these from a goroutine.
2. **mpv's render-update callback runs on an mpv thread.** It may ONLY set
   `needsRender` (atomic) and `glfw.PostEmptyEvent()` to wake the loop
   (`initMpv` in `player.go`). No mpv or GL calls from the callback.
3. **Exactly one `WaitEvent` consumer.** `forwardMpvLogs` (`mpvlog.go`) is the
   only goroutine pumping mpv's event queue — `mpv_wait_event` must not be
   called from two threads. It also handles the `idle-active` property change
   (→ `p.idle` atomic; FormatFlag arrives as int 0/1, not bool).
4. **Shutdown order is strict** (deferred in `Run`): `quit` command → wait for
   the pump to drain and exit on `EventShutdown` (3 s timeout) →
   `render.Free()` → `TerminateDestroy()`. libmpv forbids
   `terminate_destroy` while a thread is in `wait_event`, and the render
   context must be freed before terminate.
5. Cross-thread state uses the two documented atomics only: `needsRender`,
   `idle`. Everything else on `Player` is main-loop-only.
6. **The loop must exit on two conditions**, not one: `window.ShouldClose()`
   (quit action / window close button) *and* `ctx.Err() != nil` (SIGINT /
   SIGTERM). `app/cmd` installs `signal.NotifyContext`, which disables Go's
   default kill-on-SIGINT, so a loop that only checks `ShouldClose` makes
   Ctrl-C do nothing. A goroutine calls `glfw.PostEmptyEvent()` on
   `ctx.Done()` so the wait doesn't delay exit; that is the one other
   thread-safe GLFW call allowed off the main loop.

## Playlist and prefetch

- `scanPlaylist` reads the folder **once** per opened file (`setPlaylist`, from
  `Run` and `openFile`); next/previous only move `pl.idx` in the in-memory
  slice plus one `os.Stat` liveness check per candidate. Files added to the
  folder after opening deliberately do not appear — deletions are handled
  (skipped and dropped), additions are not.
- `prefetcher` (`prefetch.go`) warms the OS page cache for `pl.neighbors()`
  (next first, then previous) after every playlist change, so the demuxer finds
  the container header and trailing index in memory. It reads the first 2 MiB
  and last 256 KiB read-only and discards the bytes; nothing is written.
- Each `pf.start` cancels the previous generation, so holding down
  next/previous does not pile up reads for files the user already skipped.
  `Run` defers `pf.stop()` so no warm-up outlives the player.
- The warm goroutines touch **no** `Player` state — that is what keeps them
  compatible with threading invariant 5. Keep it that way: if a prefetch ever
  needs to report back, route it through an atomic or the loop, not by writing
  to `Player` fields. `prefetcher.warm` is injectable purely so tests can
  record paths instead of doing I/O.
- Expected payoff is storage-dependent, and measuring is the only way to know:
  on a local SSD with a warm cache the cold header read is ~2 ms, so this saves
  little; on HDD, USB, or network mounts (NFS/SMB/sshfs) the same read is the
  dominant switch cost. Do not "optimize" this away based on a fast local disk
  — the same reasoning as the hwdec probe order in `AGENTS.md`.

## Rendering invariants

- **Backbuffer stays linear** — mpv outputs sRGB-encoded pixels already; an
  sRGB-encoding framebuffer would gamma-encode twice (washed-out colors).
  Desktop GL (macOS, `desktopGL`): request `SRGBCapable`, but leave
  `GL_FRAMEBUFFER_SRGB` disabled; Gio toggles it only during its own pass.
  GLES (Linux/Windows via EGL): don't request an sRGB surface at all; Gio
  gamma-corrects through its own intermediate FBO. Fixed in commit 8d55a47 —
  don't regress it.
- Frame order in `frame()`: clear → `RenderGL` (flipY=true, whole viewport) →
  Gio `Frame` composites overlay → `SwapBuffers` → `ReportSwap()`.
- The loop renders **every iteration** (capped by `WaitEventsTimeout(0.05)`),
  not only on video frames, so overlays stay responsive while paused/idle.
- macOS quirks: desktop GL 3.3 core (Gio expects ES elsewhere), default VAO
  required for the forward-compatible profile, cursor positions manually
  scaled by content scale (`input.go`).

## Input dispatch

GLFW callbacks forward pointer events to Gio's router; unconsumed events fall
through to player behavior (`registerCallbacks` in `input.go`):

- Key press → Esc special-casing first (close overlay → exit fullscreen →
  fall through), then `keyChord` lookup in `p.keymap` → registry `fn`
  (`dispatchKey`). Lock modifiers are stripped (`relevantMods`).
- `glfw.Repeat` events (key held down) reach `dispatchKey` too, but only fire
  actions whose registry `repeat` interval is non-zero, and no more often than
  that interval (`p.lastRepeat` per action). The throttle is not optional: the
  repeat rate is an OS setting, so an unthrottled next-video would race through
  a folder. Esc/confirm handling stays press-only, so holding Esc cannot
  double-confirm a delete. **Never give a destructive or toggling action a
  repeat interval** — quit, delete, trash, play-pause, fullscreen and mute must
  stay press-only.
- While a preferences binding slot is capturing, the preferences window's key
  callback routes to `handleCaptureKey` (`overlay_prefs.go`) before anything
  else: Esc cancels, every other key binds (validated by round-tripping
  `chordLabel` → `parseChord`, so only representable keys are accepted).
  Backspace and Delete are bindable like any other key — clearing a slot is the
  per-slot `×` button, not a reserved key.
- Right-click press → `openMenu(cursor)` — rebuilds `menuItems` from the
  registry + live `track-list` every open.
- Primary click: if Gio consumed it (`router.WakeupTime()` set), do nothing;
  else if menu open, close menu; else toggle pause.
- Primary-click fall-through is suppressed while the preferences overlay is
  open (clicking the panel background must not toggle pause).
- Keys still dispatch while overlays are open (no text inputs, no focus
  juggling — a design choice, keep it).

## Overlays and menu

- `overlayKind`: exactly one of info/help/menu/confirm/prefs open; opening one closes the
  others (`toggleOverlay`). All panels share `panelBG` (alpha `0xb0`) via
  `drawPanel`/`rowGrid` in `overlay.go`.
- The status flash (`osd.go`) is *not* an `overlayKind`: it is time-bounded, not
  toggled, and coexists with whatever overlay is open. `layoutUI` draws it
  before the overlay switch so a panel wins any shared pixel. It needs no
  invalidation because the loop already renders every `idleFrame`; if that ever
  becomes event-driven, the flash needs a wakeup. `layout.S` only relaxes
  `Min.Y`, so `layoutOSD` zeroes `Constraints.Min` — otherwise the panel reports
  full window width and sits flush left instead of centered.
- Info overlay reads mpv props into `infoCache` at most once per second
  (loop-driven, `player.go`); cache is dropped when closed.
- Help overlay derives rows from `defaultActions()` + effective keymap at
  layout time (`helpRows`); `chordLabel` must render names users can type
  back into config — keep it in sync with the parser's `namedKeys`.
- Preferences overlay (`overlay_prefs.go`): rows rebuilt from the registry on
  open; `applyBinding` recomputes overrides, `restoreDefaults` drops one, and
  both go through `commitOverrides`, which validates with `buildKeymap`
  (conflicts rejected atomically), rebuilds dispatch, and persists through
  `Config.SaveShortcuts` — a callback injected by `app/cmd` so the player
  stays YAML-free. Overrides equal to defaults collapse (removed from
  config).
- Binding slots are positional: `"none"` is an empty *slot*, not just a
  whole-action marker (`isNoneKey` in `keymap.go`), so clearing the primary
  writes `["none", "k"]` and the secondary stays in slot 2 instead of being
  promoted. `effectiveKeys` maps `"none"` → `""` and trims the all-empty tail,
  so `["none"]` still means fully unbound and index 0 is always the primary.
- Each binding slot has a clear button whose meaning depends on slot state
  (`clearKind`): `×` unsets a bound slot, and once the slot is empty the same
  button becomes `←`, restoring that action's defaults — that is the
  restore-to-default affordance, deliberately not a second button per row.
  Button glyphs must exist in the Go fonts (most symbol code points, e.g.
  U+2715 `✕` or U+21BA `↺`, do not and render blank).
- Menu: `layoutMenu` clamps to the window; hovered parents open submenus
  beside the column, flipping left at the right edge. Selection runs
  `onSelect` then `closeMenu()`.

## On-video widgets (pattern kept for later, currently unused)

The player used to draw a translucent play/pause button over the video; it
was removed 2026-07-25 (playback is keyboard/menu driven for now) but the
pattern is proven and will come back for richer controls. To draw a clickable
widget over the video, give `Player` a `widget.Clickable` field, then in
`layoutUI` (`ui.go`) after the idle branch:

```go
if p.playBtn.Clicked(gtx) { // consume clicks BEFORE Layout (see menu.go)
    p.togglePause()
}
dims := layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
    return layout.Inset{Bottom: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
        gtx.Constraints.Min.X = 0
        gtx.Constraints.Max.X = gtx.Dp(unit.Dp(160))
        btn := material.Button(p.theme, &p.playBtn, label) // label: "Play"/"Pause" from p.paused
        btn.Background = color.NRGBA{A: 0xb0}              // panelBG alpha
        btn.Inset = layout.UniformInset(unit.Dp(12))
        return btn.Layout(gtx)
    })
})
```

That is all: `frame()` already routes pointer events through Gio's router,
and the primary-click fall-through in `input.go` checks
`router.WakeupTime()`, so clicks the widget consumes don't also toggle pause.

## Error-handling policy

Startup errors (bad config, GL/mpv init) are fatal. Runtime mpv
property/command errors are logged at error level and the UI stays up;
unavailable info fields render `—`; unplayable dropped files log and return
to idle; a bad custom placeholder warns and falls back to the embedded PNG.
