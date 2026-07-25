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
| Size/bitrate/rate formatters | `app/player/format_test.go` | table-driven |
| Help-row generation, chord labels | `app/player/overlay_help_test.go` | table-driven |
| Menu model building | `app/player/menu_test.go` | builds `Player{}` without a window — `buildMenu`/`trackSubmenu` tolerate `p.mpv == nil`; preserve that |
| Placeholder load fallback | `app/player/idle_test.go` | temp files |
| Config load, name validation, defaults merge | `app/cmd/config_test.go` | temp YAML files |
| CLI flags/help/version | `app/cmd/root_test.go` | cobra command execution |
| Log levels, mpv level mapping | `internal/logging/logging_test.go` | table-driven |

**Convention:** anything computing *what* to display or *how* to interpret
data lives in a function with no Gio/GLFW/mpv imports and gets a table-driven
test. Only the thin layout/dispatch glue stays untested.

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
