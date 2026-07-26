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
   (→ `p.idle` atomic; FormatFlag arrives as int 0/1, not bool),
   `end-file`/`start-file` (→ `p.eofPending` atomic, see auto-advance below),
   and `time-pos`/`duration` (→ `p.obsPos`/`p.obsDur` atomics, see below).
4. **Shutdown order is strict** (deferred in `Run`): `quit` command → wait for
   the pump to drain and exit on `EventShutdown` (3 s timeout) →
   `render.Free()` → `TerminateDestroy()`. libmpv forbids
   `terminate_destroy` while a thread is in `wait_event`, and the render
   context must be freed before terminate.
5. Cross-thread state uses the documented atomics only: `needsRender`, `idle`,
   `eofPending`, `obsPos`, `obsDur`. Everything else on `Player` is
   main-loop-only.
6. **Never read an mpv property synchronously from the main loop.**
   `mpv_get_property` waits for the playback core, which after a seek is busy
   re-syncing: the call then blocks for mpv's internal 200 ms render timeout,
   `RenderGL` cannot run, and mpv logs
   `mpv_render_context_render() not being called or stuck.` while dropping the
   frames it had queued — the video visibly stutters on every seek. This is the
   documented hazard in `render.h` ("if you call normal libmpv API functions on
   the renderer thread, deadlocks can result … made non-fatal with timeouts").
   Playback position and duration are therefore **observed**, not read:
   `observePlayback` (`playback.go`) registers them in `initMpv`, the pump
   caches them via `notePlaybackProp`, and the loop reads `playbackPos()` /
   `playbackDur()`. Measured on a 180 s 720p clip: 7 seeks produced 7 stuck
   warnings and 200 ms stalls before, 0 and none after. `propFloat` and friends
   are fine for the info overlay (once a second, no flash) but must not move
   into any per-frame path — that includes the control bar, whose knob follows
   playback through `syncProgressKnob`. mpv sends a property change with **no
   value** when one becomes unavailable (both do on stop), which
   `notePlaybackProp` maps to 0 so a stale position cannot survive into the
   next file.
7. **The loop must exit on two conditions**, not one: `window.ShouldClose()`
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
- **Auto-advance on end of file** (`autoadvance.go`): the event pump flags
  `eofPending` when mpv reports `end-file`, and the loop calls
  `handleEndOfFile` → `playAdjacent(1)`. Split that way on purpose — invariants
  2/5: `loadfile` must run on the main loop, so the pump only sets an atomic
  (plus `PostEmptyEvent` so the switch doesn't wait out the 50 ms timeout).
  Only reason `eof` advances. `stop` is what *our own* `loadfile`/`stop`
  produce, `error` is an unplayable file (advancing on it would run through the
  whole folder as fast as mpv can fail), `quit` fires during shutdown, and
  `redirect` is a playlist being expanded. mpv reports `eof` for a seek past
  the end too, which is exactly why keyboard navigation off the end also
  chains — verified with a headless-mpv test (`TestSeekPastEndReportsEOF`), so
  don't narrow the reason check to "played through".
- **A seek off the end needs its own handling** (`seek.go`: `passesEnd`,
  `advancePastEnd`), because mpv *clamps* a seek to the last keyframe before the
  end instead of overshooting it — so it never reports end-of-file and
  auto-advance alone would leave the tail of the file playing. `seek` and
  `seekPercent` therefore check `time-pos + step >= duration` first and continue
  with the next entry instead of sending the command — from the observed values
  (threading invariant 6), never a synchronous read. Measured: a plain 5 s seek
  does overshoot and yields `eof`, while repeated 10 % seeks on a real MP4 sit at
  ~56 s of 60 s forever (`TestSeekPercentPastEndAdvancesOnRealFile` pins it with
  a real encoded file — a synthetic lavfi source does *not* reproduce the
  clamping). It deliberately falls through to mpv at the last entry, with no
  playlist, or when `duration <= 0` (live stream), where the seek is either
  harmless or mpv's idle screen is the right outcome.
- The flag is disarmed in two places, because end-of-file and the user pressing
  next can coincide: `loadFile` clears it (before the `p.mpv == nil` guard, so
  the test path is covered too) and `EventStart` clears it via `noteFileStart`
  for the reverse ordering, where the eof lands after `loadfile` already ran.
  Without both, one PageDown at the wrong moment skips two entries.
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
  otherwise `primaryClickOutcome` decides — menu open closes the menu, a second
  press inside `doubleClickWindow` toggles fullscreen, and anything else does
  **nothing**. Clicking the video to pause was removed 2026-07-26 (too easy to
  hit by accident): pause is the control bar's play button, the `play-pause`
  shortcut, and the menu entry. Don't re-add it — and note the menu branch
  deliberately outranks the double click, so dismissing a menu with a quick
  second click cannot also throw the window into fullscreen.
- The decision is a pure function returning a `clickOutcome` precisely because
  both of its effects (`closeMenu`, `toggleFullscreen`) need a real GLFW window;
  the effects are untestable glue, the decision is table-tested.
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
- **Every flash is a snapshot.** It used to have a second mode: a progress flash
  that re-read `time-pos` from the loop (`osdProgress`/`refreshOSD`) because mpv
  applies a seek asynchronously. That went away on 2026-07-26 along with
  `progressStatus` and `humanClock` — the control bar reports progress now, and
  its knob re-reads the observed position every frame anyway, so the mechanism
  had no second user. Don't reintroduce a self-updating flash for progress; if
  another live value ever needs one, the loop hook is the place, not `flash`.
- Info overlay reads mpv props into `infoCache` at most once per second
  (loop-driven, `player.go`); cache is dropped when closed. The split is
  deliberate: `overlay_info.go` owns the mpv reads (`mediaInfo()`) and the Gio
  layout, `mediainfo.go` owns the `mediaInfo` value and `infoSections` — which
  is mpv- and Gio-free, so the grouping and every fallback are table-tested
  (`mediainfo_test.go`). Add a field to `mediaInfo` and read it in
  `mediaInfo()`; don't reach for a property from the formatting side.
- Duration comes from `playbackDur()` (observed, invariant 6), not a
  `propFloat("duration")` read — the overlay would otherwise reintroduce a
  synchronous read of a property already in the cache.
- Each track kind gets a section even with zero tracks, rendering `none`: an
  absent section reads as missing data, which is a different thing from a file
  having no subtitles. Every track of a kind is listed, not just the selected
  one (`▶` marks that); a kind's non-track rows (resolution, bitrate, channels)
  follow its track rows in the same section.
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

## Control bar (on-video widgets)

`controls.go` draws the bottom bar: play button, seek bar, volume button. It is
the live example of the on-video widget pattern — `frame()` already routes
pointer events through Gio's router, and the primary-click fall-through in
`input.go` checks `router.WakeupTime()`, so clicks a widget consumes don't also
toggle pause. Points that will bite if changed:

- **Clicks are consumed before `Layout`**, like menu rows: `Clickable.Layout`
  drains pending click events, so a `Clicked` check after it never fires. The
  volume button's `Clicked` is called and discarded for the same reason — an
  unread click would otherwise queue up and fire whenever it is finally wired.
- **`widget.Float` owns the knob position, and the drag wins over playback.**
  `Update` is called first; if it reports a change the value came from the
  pointer and drives a scrub, and only when *not* dragging is `Value`
  overwritten from `time-pos`. Reversing that order makes the knob snap back to
  the playhead mid-drag.
- **The drag area is inset by `knobGrab` at both ends** (`layoutProgressBar`),
  so `Float.Value` 0..1 maps onto the knob's own travel and the circle stays
  inside the row at 0 % and 100 %. `knobGrab` is the hit radius passed to
  `Float.Layout` and also sets the row height; `knobRadius` is only the drawn
  circle, so the visible knob can shrink without shrinking the pointer target.
  Insetting by `knobRadius` instead would let the grab area overhang the row.
- **Scrubbing uses `absolute+keyframes`** (`seekAbsoluteCommand`), not `exact`:
  a drag fires a seek per pointer move, and exact seeks would decode up to every
  intermediate frame. Verified end to end — one drag produced
  `target="14.462" flags="absolute+keyframes"` … `target="50.227"`.
- **Auto-hide is state-free**: `controlsVisible(now, lastInput, dragging)`
  compares `gtx.Now` against the last pointer input, fed by `revealControls`
  from the cursor and button callbacks. No timer needed (the loop renders every
  `idleFrame`, same as the status flash); if that ever becomes event-driven, the
  bar needs a wakeup too. `dragging` overrides the timeout so a slow scrub
  cannot make the bar vanish under the pointer, and a zero `lastInput` means the
  pointer has not moved yet, so the bar starts hidden.
- **The bar is also the keyboard's progress readout.** `revealControls` is not
  pointer-only: `runSeek` (`seek.go`) calls it after mpv accepts a seek, and the
  `progress` action (`o`) is nothing but a call to it. That is what replaced the
  text flash on 2026-07-26 — no separate re-read machinery is needed, since
  `syncProgressKnob` already re-reads the observed position every frame. Note
  `runSeek` reveals *after* the command succeeds, so a rejected seek shows
  nothing; a test with an uninitialized mpv handle therefore never exercises the
  reveal (see `TestSeekRevealsTheControlBar`, which loads a real clip).
- **The fade is derived, not animated**: `controlsAlpha` computes opacity from
  the same two timestamps (`controlsFade`, 150 ms), and every colour goes through
  `fadeColor`. Three details are load-bearing. `controlsVisible`'s window is
  `controlsHideAfter + controlsFade`, or the fade-out would be cut off mid-ramp.
  `revealControls` moves `revealedAt` back by the alpha the bar already has, so
  re-revealing a half-faded bar resumes from where it is instead of flickering
  through transparent, and a continuous stream of mouse moves does not restart
  the fade-in on every event. `controlsFade` is not worth shrinking much below
  its current value: at a 50 ms render tick it already spans only ~3 frames.
- **The icons are vector paths, not text.** The Go fonts carry no media glyphs
  at all — U+25B6 `▶`, U+23F8 `⏸` and U+1F50A `🔊` are all absent (checked
  against `gofont.Collection`; so are `✓` and `▸`, which is why `menu.go`'s
  markers are the way they are). `controls.go` uses `widget.Icon` over
  `golang.org/x/exp/shiny/materialdesign/icons`, which was already an indirect
  dependency and is BSD-3 (allowlisted).
- Layer order in `layoutUI`: control bar first, then the flash, then the
  overlays — so a panel or a flash wins any pixel it shares with the bar.

## Error-handling policy

Startup errors (bad config, GL/mpv init) are fatal. Runtime mpv
property/command errors are logged at error level and the UI stays up;
unavailable info fields render `—`; unplayable dropped files log and return
to idle; a bad custom placeholder warns and falls back to the embedded PNG.
