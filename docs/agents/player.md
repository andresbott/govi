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
   `time-pos`/`duration` (→ `p.obsPos`/`p.obsDur` atomics, see below) and
   `volume` (→ `p.obsVol`).
4. **Shutdown order is strict** (deferred in `Run`): `quit` command → wait for
   the pump to drain and exit on `EventShutdown` (3 s timeout) →
   `render.Free()` → `TerminateDestroy()`. libmpv forbids
   `terminate_destroy` while a thread is in `wait_event`, and the render
   context must be freed before terminate.
5. Cross-thread state uses the documented atomics only: `needsRender`, `idle`,
   `eofPending`, `obsPos`, `obsDur`, `obsVol`. Everything else on `Player` is
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
   next file. **`volume` is observed for the same reason** (`observeVolume` /
   `noteVolumeProp` / `volumeLevel()` in `volume.go`, userdata 4): the bar's
   volume slider and its button glyph both read the level on every frame the bar
   is visible. The one synchronous `propInt("volume")` left is in `noteVolumeChanged`,
   which runs off a key press rather than per frame — and it publishes what it
   read into the cache, so the slider does not sit stale until mpv's own
   notification lands a frame or more later.
7. **The loop must exit on two conditions**, not one: `window.ShouldClose()`
   (quit action / window close button) *and* `ctx.Err() != nil` (SIGINT /
   SIGTERM). `app/cmd` installs `signal.NotifyContext`, which disables Go's
   default kill-on-SIGINT, so a loop that only checks `ShouldClose` makes
   Ctrl-C do nothing. A goroutine calls `glfw.PostEmptyEvent()` on
   `ctx.Done()` so the wait doesn't delay exit; that is the one other
   thread-safe GLFW call allowed off the main loop.
8. **macOS open-document events must not call mpv** (`openfiles_darwin.go`).
   `-application:openFiles:` runs on the Cocoa main thread inside GLFW's event
   pump, so it only puts the path in a buffered channel and posts a wake-up
   event; the loop picks it up through `takePendingOpen` and calls `openFile`.
   Same split as auto-advance, for the same reason (invariants 1/2), with one
   extra: on a **cold start** the event fires while `initWindow` finishes
   AppKit's launch sequence, before mpv exists at all, so a direct call would
   dereference a nil handle. The wake-up is `goviWakeEventLoop`, not
   `glfw.PostEmptyEvent`, which would open a nested `[NSApp run]` at that point.
9. **`installOpenFilesHandler` also finishes AppKit's launch, and its failure is
   fatal.** It sits between `glfw.Init` and `glfw.CreateWindow` in `initWindow`,
   and both bounds are load-bearing. Without it a bundled govi never opens a
   window from a shell at all, because `glfw.CreateWindow` blocks in
   `[NSApp run]` waiting for a launch notification that only arrives on
   activation. See the two macOS sections of [releasing.md](releasing.md).

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
  re-bind `p.defVAO` (desktop GL only) → Gio `Frame` composites overlay →
  `SwapBuffers` → `ReportSwap()`.
- **A VAO must be bound before Gio draws** — mpv ends every render with
  `glBindVertexArray(0)` (its `gl_vao_unbind`), and Gio shares this context, so
  its per-frame state save/restore calls `glVertexAttribPointer`, which is
  illegal with no VAO bound in a core profile. The `GL_INVALID_OPERATION` then
  waits in the context until mpv's next `glGetError` drain reports it as
  `after creating texture: OpenGL error INVALID_OPERATION` — an error attributed
  to mpv that mpv never caused, and macOS-only because only `desktopGL` uses a
  core profile. Binding the VAO once at init is **not** enough: mpv unbinds it
  again on every frame. Don't move the re-bind out of `frame()`.
- The loop renders **every iteration** (capped by `WaitEventsTimeout(0.05)`),
  not only on video frames, so overlays stay responsive while paused/idle.
- macOS quirks: desktop GL 3.3 core (Gio expects ES elsewhere), default VAO
  required for the forward-compatible profile (see the invariant above), cursor
  positions manually scaled by content scale (`input.go`).

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
  back into config — keep it in sync with the parser's `namedKeys`. A row keeps
  its two bindings apart (`primary`/`secondary`) rather than joining them into
  one `"space [k]"` string, so the panel can column them and label the slots the
  way the preferences window does; a keyless action's `menu`/`unbound` marker
  goes in `primary`, where the eye looks for a binding.
- **The help panel is two columns, and the split is by half, not alternating**
  (`splitHelpRows`): the left column is the first half of the registry, so
  reading down then across preserves declaration order. 19 actions in one list
  is 340 dp tall against 187 dp split — the reason it exists (both measured in
  `TestHelpPanelIsShorterInTwoColumns`, which also pins that the split trades
  height for width rather than growing both).
- **The help cells are fixed widths, so overflow is silent.** Each of the three
  (`helpLabelW`, `helpKeyW`, `helpAltKeyW`) carries slack over the widest string
  it can hold; a longer action label or a newly nameable key is exactly the
  change that would wrap or clip one, which is why
  `TestHelpCellsFitTheirWidestText` measures every label, every default binding
  and every entry in `namedKeys` against its column. Widen the constant rather
  than shortening a label. Note both columns repeat the `primary`/`secondary`
  heading — a single heading over the left one reads as covering the whole panel.
- Measuring a panel in a test needs `Constraints.Min` zeroed: with `Min == Max`
  (what `layout.Exact` gives) a `Flex` fills its parent and every measurement
  comes back as the window size. `layout.Center` does that relaxing in the real
  path, which is why `layoutHelp` itself cannot be measured directly.
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

`controls.go` draws the bottom bar: play button, seek bar, mute button, volume
slider. It is the live example of the on-video widget pattern — `frame()` already
routes pointer events through Gio's router, and the primary-click fall-through in
`input.go` checks `router.WakeupTime()`, so clicks a widget consumes don't also
toggle pause. Points that will bite if changed:

- **All pointer handling lives in `updateControls`**, called once at the top of
  `layoutControls` and *before* any widget's `Layout`. It is split out so the
  wiring is testable without the flex geometry (`controls_ui_test.go` drives it
  through a real `input.Router` with no display).
- **Clicks are consumed before `Layout`**, like menu rows: `Clickable.Layout`
  drains pending click events, so a `Clicked` check after it never fires. This is
  the ordering `updateControls` exists to guarantee for both buttons.
- **`widget.Float` owns each knob's position, and the drag wins over the
  property.** Per slider, `Update` is called first; if it reports a change the
  value came from the pointer and drives the effect (scrub / set volume), and only
  when *not* dragging is `Value` overwritten from mpv (`time-pos` / `volume`).
  Reversing that order makes the knob snap back mid-drag.
- **The two sliders are the same widget** (`layoutSlider`), so a volume drag
  behaves exactly like a scrub; only the width differs, since both share
  `filledColor`. Keep their `Update`/`Value` pairs distinct — wiring the volume
  `Float` to `scrub` (or vice versa) would jump playback on a volume change, which
  is what `TestPressingTheVolumeSliderLeavesTheProgressKnobAlone` pins.
- **`barDragging()` is either slider**, not just `progress`: it feeds
  `controlsVisible`/`controlsAlpha`, so a slow volume drag must hold the bar up
  the same way a slow scrub does.
- **The drag area is inset by `knobGrab` at both ends** (`layoutSlider`), so
  `Float.Value` 0..1 maps onto the knob's own travel and the circle stays
  inside the row at 0 % and 100 %. `knobGrab` is the hit radius passed to
  `Float.Layout` and also sets the row height; `knobRadius` is only the drawn
  circle, so the visible knob can shrink without shrinking the pointer target.
  Insetting by `knobRadius` instead would let the grab area overhang the row.
- **The volume slider is `Rigid` at a fixed `volumeBarWidth`** (120 dp), while the
  seek bar is the row's only `Flexed` child. The volume range is 0..`volumeMax` at
  any window size, and a viewport-wide volume slider would read as the more
  important of the two. Since both sliders now share the same blue fill, that width
  difference and their positions in the row are the *only* things distinguishing
  them — don't make the volume one flexed.
- **`volumeMax` and mpv's `volume-max` option must stay equal** — `initMpv` takes
  the option string from `volumeMaxOption()` so they cannot drift. The slider maps
  its full travel onto 0..`volumeMax`: a larger mpv ceiling leaves part of the
  range unreachable, and a smaller one lets the knob ask for a level mpv silently
  clamps, so the knob then sits where the volume isn't.
- **`volumeTarget` rounds to a whole percent.** The shortcuts step in whole
  percents and the saved level is an int, so a drag landing on 44.999… (which is
  what `float32(0.45)` scales to) would persist and reload as a value the keyboard
  can never reproduce. mpv itself accepts fractions.
- **The volume button is a mute toggle, and its glyph is the mute/level state** —
  not what the click will do, unlike the play button. That is the convention every
  other player uses, and the level half of it is why `volume` has to be observed:
  `volumeIcon` reads it per frame.
- **The level and mute flag persist across runs, debounced.** Every change path
  arms `volumeSavePending` (`noteVolumeChange`, called from `noteVolumeChanged` for
  the keyboard/menu and from `setVolume` for the drag, which already knows the level
  it just set), and the loop calls `saveVolumeIfDue` — which writes only once the level
  has been still for `volumeSaveDelay` (500 ms). The debounce is not optional: a
  drag fires a set-volume per pointer move, so an immediate save would rewrite
  config.yaml dozens of times per drag. `loop` also defers `flushVolumeSave`, since
  a change made inside the last 500 ms before quitting would otherwise be lost, and
  nothing waits out the delay on the way down. Both go through `saveVolumeNow`,
  which **disarms before running the callback and on every outcome** — an armed
  save that survived its own attempt would retry each frame, which for a failing
  write means flooding the log. Nothing pending writes nothing, so merely opening
  and closing the player does not rewrite the file.
- **The restore is an mpv option, not a property.** `mpvOptions` sets `volume`
  before `Initialize` so playback *starts* at the saved level instead of stepping
  up to it audibly over the first frames; mute is a property, so `applyAudioState`
  runs right after `initMpv`. `mpvOptions` is split out of `initMpv` purely because
  `initMpv` builds a GL render context and so needs a GLFW context no display-free
  test has.
- **An unset level must stay distinct from a saved 0.** `Config.Volume` is a
  `*int`: nil leaves mpv's own default alone, so a fresh install does not start
  either muted or at full blast. `app/cmd` carries the same distinction as the
  `volumeUnset` sentinel — a pointer field there would not work, because bumbu
  allocates nil pointers before unmarshalling and an absent key would come back as
  a set 0 (see `config.go`). A hand-edited out-of-range level is clamped by
  `startVolume`, not rejected.
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
  `progress` action (`o`) reveals it too. That is what replaced the text flash on
  2026-07-26 — no separate re-read machinery is needed, since
  `syncProgressKnob` already re-reads the observed position every frame. Note
  `runSeek` reveals *after* the command succeeds, so a rejected seek shows
  nothing; a test with an uninitialized mpv handle therefore never exercises the
  reveal (see `TestSeekRevealsTheControlBar`, which loads a real clip).
- **`o` toggles, and `hideControls` is the mirror of `revealControls`.** Because
  the bar *is* the readout, the key that summons it has to put it away — waiting
  out the 1 s auto-hide is the only alternative for a keyboard-only user.
  `revealControls` moves `revealedAt` back by the alpha the bar already has;
  `hideControls` moves `lastInput` back past `controlsHideAfter` by the
  complement, so the fade-out resumes at the current alpha instead of snapping to
  opaque and starting over. Zeroing `lastInput` would also hide the bar, but
  instantly, and a later reveal would then fade in from nothing.
- **`controlsShowing` is not `controlsVisible`** — it is the narrower "up and
  staying up" (inside the hold window, fade-out excluded), and it is what
  `toggleProgress` decides on. Deciding on `controlsVisible` would make a press
  during the fade-out re-hide a bar that is already leaving, so the key would
  appear to do nothing. `controlsVisible` stays the drawing predicate; don't
  merge them.
- **The same holds for volume**: `noteVolumeChanged` reveals the bar, so
  `up`/`down`, `+`/`-`, `m` and the menu entries all show the slider moving (and
  the glyph changing, for mute). It draws **no text** — the `Volume 45%` / `Muted`
  flash was removed on 2026-07-26 along with `volumeStatus`, exactly as the progress
  flash went; the bar is the whole readout. Don't reintroduce it. `changeVolume` and
  `toggleMute` both call `noteVolumeChanged`, so that is the single place the reveal,
  the mpv read-back and the save are wired.
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
  dependency and is BSD-3 (allowlisted). The three volume glyphs live in
  `volume.go` next to the state that selects between them, not with the
  play/pause pair.
- Layer order in `layoutUI`: control bar first, then the flash, then the
  overlays — so a panel or a flash wins any pixel it shares with the bar.

## Mouse-pointer auto-hide (`cursor.go`)

The pointer hides itself in fullscreen after `cursorHideAfter` (3 s) of a still
mouse, and comes back on the next move. Same shape as the control bar's
auto-hide, with four deliberate differences.

- **`lastPointerMove` is not `lastInput`.** The bar's timestamp also moves on
  clicks and on the seek/volume *keyboard* actions (`revealControls`), and a
  keyboard seek must not put a hidden pointer back on screen. Only a real cursor
  move touches `lastPointerMove`. Don't collapse the two fields.
- **Fullscreen only.** In a window the pointer is how the user reaches the title
  bar, the other windows and the rest of the desktop; fullscreen is the one state
  where it sits over the picture with nothing to do. `fullscreen()` asks GLFW
  (`GetMonitor() != nil`) rather than tracking a flag, so it cannot drift from
  what `toggleFullscreen` did.
- **3 s, not the bar's 1 s.** A bar going away is undone by moving a
  millimetre; a vanished pointer is briefly disorienting, so it waits until the
  mouse is clearly out of use. Reconciled from the loop for the same reason as
  the bar — the loop already ticks every `idleFrame`, so no timer — but note the
  asymmetry: the *hide* is loop-driven (nothing moves when the mouse is the thing
  that stopped), while the *reveal* happens in the cursor callback via
  `handlePointerMove`, because it has to feel simultaneous with the movement
  rather than up to a frame late.
- **`pointerBusy` pins it visible**: a knob held still mid-drag or an open
  context menu is a pointer in use, not an idle one. It reuses the bar's
  `barDragging` so the two auto-hides agree on what a drag is. The other overlays
  (info, help, confirm) are keyboard-driven and don't count.

`syncCursor` calls GLFW only on a change, so the steady state is free, and
`applyCursorMode` uses `CursorHidden` rather than `CursorDisabled`: the pointer
is invisible over the window but still moves normally and can leave for another
screen. `CursorDisabled` would grab it and switch GLFW to relative motion,
breaking every widget in the bar.

## Error-handling policy

Startup errors (bad config, GL/mpv init) are fatal. Runtime mpv
property/command errors are logged at error level and the UI stays up;
unavailable info fields render `—`; unplayable dropped files log and return
to idle; a bad custom placeholder warns and falls back to the embedded PNG.
