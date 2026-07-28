# Testing — strategy, gates, conventions

See [architecture.md](architecture.md) for layering. The strategy in one line:
**pure logic is extracted into Gio/mpv-free functions and unit-tested; the
GL/mpv/GLFW glue is manually tested** (needs a display, consistent with the
codebase's origins in the 2026-07-23 design spec).

## Commands / gates

```sh
make test       # go test ./... -cover
make lint       # golangci-lint (config: .golangci.yaml)
make coverage   # per-package threshold check, default 70%
make benchmark  # go test -bench (none exist yet; gate still runs)
make verify     # test + license-check + lint + benchmark + coverage — run before finishing
```

CI (`.github/workflows/`): test, golangci-lint, and license-check run on push
to main and on PRs. `release.yml` runs on `v*.*.*` tags — see
[releasing.md](releasing.md).

## Coverage gate specifics

`make coverage` checks every package under `app/...` against
`COVERAGE_THRESHOLD` (70%), **excluding** two packages by design:

- `app/metainfo` — linker-stamped vars only, nothing to test.
- `app/player` — GL/mpv glue needing a display. Its testable logic is instead
  kept in display-free files (see below); don't let the exclusion tempt you
  into skipping tests for extractable logic.

`internal/...` is not covered by the gate (only `app/...` is listed), but
`internal/logging` has tests anyway.

## What is unit-tested (keep new logic in this shape)

| Area | File | Style |
|---|---|---|
| Key-string parser, keymap merge/dup detection | `app/player/keymap_test.go` | table-driven (`tests := []struct{...}`) |
| Track-list parsing, zoom/aspect mapping | `app/player/tracks_test.go` | table-driven |
| Size/bitrate/rate/duration formatters, path splitting | `app/player/format_test.go` | table-driven |
| Info-overlay grouping: sections, per-kind track lists, `—`/`none` fallbacks | `app/player/mediainfo_test.go` | builds a `mediaInfo` literal and asserts on `infoSections` — no mpv handle, which is what keeps the grouping mpv-free |
| Help-row generation (primary/secondary split), chord labels, the two-column split | `app/player/overlay_help_test.go` | table-driven, plus two measurement tests |
| That the help panel's fixed cells fit their widest text and the split really is shorter | `app/player/overlay_help_test.go` (`TestHelpCellsFitTheirWidestText`, `TestHelpPanelIsShorterInTwoColumns`) | lays Gio out with a real font shaper but no display or GPU — text shaping is pure measurement. Zero `Constraints.Min` first, or a `Flex` fills its parent and every size comes back as the window |
| That `o` reveals the control bar, hides it again on a second press, and fades rather than cuts | `app/player/controls_test.go` | a bare `Player{}`: `toggleProgress` only moves timestamps, so no mpv handle is needed |
| Pointer auto-hide: fullscreen-only, the 3 s delay, the busy guard, and that a move brings back both the pointer and the bar | `app/player/cursor_test.go` | a bare `Player{}` with no window: `cursorHidden` is pure, and `applyCursorMode` no-ops without one — the assertions are on `p.cursorHidden`, not on GLFW |
| Menu model building | `app/player/menu_test.go` | builds `Player{}` without a window — `buildMenu`/`trackSubmenu` tolerate `p.mpv == nil`; preserve that |
| Placeholder load fallback | `app/player/idle_test.go` | temp files |
| Observed position/duration cache, and that the seek and control-bar paths use it | `app/player/playback_test.go`, `playback_controls_test.go` | a `Player{}` with **no** mpv handle is the assertion: a synchronous property read would panic, which is how "never read mpv from the render thread" (player.md invariant 6) stays pinned |
| That every keyboard route to progress reveals the bar and flashes no text | `app/player/seek_end_mpv_test.go` (`TestKeyboardProgressActionsRevealBarNotText` and the two per-action tests) | needs a real headless mpv with a clip loaded: `runSeek` reveals only after mpv accepts the command, so an uninitialized handle would pass the test vacuously |
| Config load, name validation, defaults merge | `app/cmd/config_test.go` | temp YAML files |
| CLI flags/help/version | `app/cmd/root_test.go` | cobra command execution |
| Log levels, mpv level mapping | `internal/logging/logging_test.go` | table-driven |

**Convention:** anything computing *what* to display or *how* to interpret
data lives in a function with no Gio/GLFW/mpv imports and gets a table-driven
test. Only the thin layout/dispatch glue stays untested.

**Headless-mpv helpers** (`seek_end_mpv_test.go`): `headlessPlayer` registers the
playback observers the way `initMpv` does, and `waitPlaying` routes property
changes into the cache as it drains the queue — standing in for the event pump,
which these tests do not run (it calls `glfw.PostEmptyEvent` on end-file, and
there is no GLFW here). A test that seeks without going through `waitPlaying`
sees a position of 0. Don't run `forwardMpvLogs` alongside `headlessPlayer`
either: its cleanup drains the queue too, and `mpv_wait_event` forbids two
consumers — build the handle inline instead (see
`TestPumpRoutesObservedPlaybackProps`).

## Lint notes

`.golangci.yaml` enables nolintlint (explanations + specific linter
required), gocyclo (min 20), nestif, gosec, dupl. Test files are excluded
from nestif/dupl/gosec. `slog` nil-context calls carry
`//nolint:staticcheck // slog accepts a nil context` — reuse that exact
pattern.

## Manual testing

`make run FILE="path"` runs with `--log debug`. mpv's own logs are forwarded
as `component=mpv`; debug/verbose level shows hwdec probing, audio init, and
first-frame timing — the primary tool for startup-latency work. `--log trace`
adds per-frame render timing.
