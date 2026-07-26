# Architecture — govi, a minimalistic mpv-based video player

Single binary, Go 1.26, GPL-3.0. libmpv renders video into a GLFW-owned GL
context; Gio draws all UI (overlays, idle screen, context menu) into the same
framebuffer, so frames never leave the GPU. "Minimalistic" scopes **features,
not implementation** — complex internals (custom menu widget, action registry)
are deliberate and fine.

Related docs: [features.md](features.md) · [player.md](player.md) ·
[testing.md](testing.md) · [releasing.md](releasing.md). Hardware-portability
rules live in the repo-root `AGENTS.md` and are **inviolable** (short version:
never drop or pin hwdec/audio/windowing probing to the local machine's vendor;
the CUDA probe's ~650 ms failure on non-NVIDIA machines is accepted cost).

## Layering

```
main.go → app/cmd (cobra CLI, config load/validate)
              ↓ translates AppCfg → player.Config
          app/player (GLFW + GL + libmpv + Gio; everything user-visible)
          app/metainfo (linker-stamped version vars only)
          internal/logging (slog + trace level + mpv level mapping)
          libs/trash (OS trash; no govi dependencies, reusable standalone)
```

**Invariant: `app/player` must NOT import bumbu or YAML.** All config-library
knowledge stays in `app/cmd`; `AppCfg.toPlayerConfig()` produces the plain
`player.Config` (`app/cmd/config.go:97`). Keep it that way.

## Key decisions (and why)

- **Action registry is the single source of truth** (`app/player/actions.go`).
  One table of `action{id, label, defaults, fn}` feeds three consumers: the
  keymap (dispatch), the help overlay (rows), and the context menu (entries).
  Never add a shortcut, menu entry, or help row that bypasses the registry —
  the whole design exists so these can't drift apart.
  **Adding an action means three edits in `app/cmd/config.go` too**, none of
  which the compiler enforces: a `ShortcutsCfg` field, an entry in
  `knownActions` (or the config is rejected as a typo), and an `add(...)` call
  in `toPlayerConfig` (or the override loads and is silently dropped).
  `TestEveryKnownActionReachesPlayerConfig` covers the last two.
- **sRGB double-gamma fix** (commit 8d55a47): the backbuffer stays *linear*
  because mpv already outputs sRGB-encoded pixels. Desktop GL (macOS) and GLES
  (everywhere else) handle this differently — see the long comments in
  `app/player/player.go` `initWindow`. Do not "clean up" the sRGB hints or
  enable `GL_FRAMEBUFFER_SRGB` globally; colors will wash out.
- **hwdec is a priority list, not "auto"** (`initMpv`): cheap-to-fail probes
  first so CUDA's slow failure only runs when nothing faster matched, with a
  trailing `auto` for coverage. Reordering for latency is fine; removing
  vendors is not (AGENTS.md).
- **Config defaults are applied player-side, not via bumbu `Defaults`**:
  bumbu overlays list values per index, which would merge a one-key user
  override with the default's second key. Instead `cmd` passes only what the
  user set, and `buildKeymap` (`app/player/keymap.go`) merges wholesale per
  action. `"none"` unbinds an action.
- **Shortcuts are a struct, not a map, in `AppCfg`**: bumbu cannot unmarshal
  into Go maps. Unknown action names are validated separately against the raw
  YAML (`validateShortcutNames`) because bumbu silently drops unknown fields.
  Bad config is startup-fatal, naming the offending entry; a missing file is
  silent defaults.
- **Two writers share config.yaml**, so both go through `updateConfig`, which
  read-modify-writes the file as a whole YAML document and leaves every other
  top-level key intact: `saveShortcuts` (preferences window) owns `shortcuts:`, and
  `saveAudioState` (volume, debounced in the render loop) owns `volume:`/`muted:`.
  A writer that marshalled only its own section would silently drop the other's —
  `TestSaveShortcutsPreservesTheSavedVolume` and its mirror pin that. Comments are
  not preserved by either; writes are atomic (temp file + rename).
- **"Unset" needs a sentinel, not a nil pointer, in `AppCfg`**: bumbu allocates
  nil pointer fields before unmarshalling, so a `*int` would come back as a set 0
  for a config that never mentions the key — and a saved volume of 0 is a real,
  different thing (silence). `Volume` is therefore an `int` seeded with
  `volumeUnset` by `loadConfig`, converted to `player.Config`'s `*int` in
  `toPlayerConfig`. The player side *can* use a pointer, since nothing reflects
  over it.
- **Trash is per-OS native, and never copies** (`libs/trash`): Windows uses
  `SHFileOperationW` with `FOF_ALLOWUNDO`, macOS `-[NSFileManager
  trashItemAtURL:]` via cgo (the only way to get Finder's "Put Back"), Linux/BSD
  the FreeDesktop spec *including* per-volume `.Trash-$uid` — so trashing from a
  Samba/NFS/USB mount is a rename on that mount, not a copy to `$HOME`. A
  cross-device trash **fails** rather than transferring data, which is what keeps
  `moveToTrash` safe to call synchronously on the main loop. The previous
  dependency (`hymkor/trash-go`) did the opposite: home-trash only, silent
  copy-then-delete across devices, and it reported success when the source
  could not be unlinked.
- **Threading model** — three rules, all load-bearing (details in
  [player.md](player.md)): the main OS thread is locked for GLFW/GL; mpv's
  render callback only sets an atomic flag and posts an empty event; one
  goroutine is the sole `WaitEvent` consumer and shutdown is strictly
  quit → drain pump → `render.Free()` → `TerminateDestroy()`.

## Domain types worth knowing

- `player.Player` — all runtime state (one instance, main-loop owned except
  the documented atomics `needsRender`, `idle`, `eofPending`, `obsPos`, `obsDur`,
  `obsVol`).
- `actionID` / `action` — registry entry; IDs double as YAML config keys.
- `keyChord{glfw.Key, ModifierKey}` — dispatch unit; lock modifiers stripped.
- `overlayKind` — `none | info | help | menu | confirm | prefs`; exactly one overlay at a time.
- `menuItem` — separator, leaf (`onSelect`), or parent (`sub`); tree rebuilt
  from mpv `track-list` on every right-click.
- `trackInfo` — normalized mpv track entry (`app/player/tracks.go`), no Gio
  dependency so it's unit-testable.

## History / decision records

- Design spec: `docs/superpowers/specs/2026-07-23-player-usability-design.md`
  (approved 2026-07-23). Split into four plans under `docs/superpowers/plans/`.
- **All four plans are fully implemented** (commits of 2026-07-24, `b45a5af`
  through `8cea6f1`) even though the plan files' `- [ ]` checkboxes were never
  ticked. The code is the truth; don't re-execute the plans.
- Spec drift: the spec's `config.Load` sketch included a `config.Defaults`
  step — the implementation deliberately dropped it (see defaults decision
  above). The spec's file map is otherwise accurate, plus `format.go`,
  `tracks.go`, `overlay.go`, `mpvlog.go`, `ui.go` grew as separate files.

## Known debt / deferred

Deliberate gaps and their status are catalogued in [features.md](features.md)
— check there before adding a capability.
