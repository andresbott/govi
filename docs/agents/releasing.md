# Releasing — goreleaser via tag push

See [testing.md](testing.md) for the verification gates; run `make verify`
before tagging.

## Flow

```sh
make tag version="v0.1.0"   # requires: main branch, clean tree
```

`make tag` deletes any existing local/remote tag of that name, creates an
annotated tag, and pushes it. The push triggers
`.github/workflows/release.yml` (`v*.*.*` and `v*.*.*-*` prerelease tags),
which runs goreleaser.

## Build constraints (`.goreleaser.yaml`)

- **Linux amd64 only, CGO_ENABLED=1** — govi links libmpv via cgo, so no
  cross-compiled darwin/windows targets and never `CGO_ENABLED=0`. Adding a
  platform means adding a native build runner, not a goarch line.
- Version info is linker-stamped into `app/metainfo` (`Version`, `BuildTime`,
  `ShaVer`); dev builds show `dev-build` and stamp `BuildTime` at init.
- Artifacts: tar.gz archive (uname-compatible naming) plus nfpm packages.

## Desktop integration (`.deb` only)

The nfpm package installs `zarf/govi.desktop` to
`/usr/share/applications/` and the icons from `zarf/govi.svg` /
`govi-<size>.png` into `/usr/share/icons/hicolor/`, which is what makes govi
appear in the launcher and in the file manager's "Open with" list.

- The `MimeType=` list in `govi.desktop` is the only place that decides which
  formats offer govi as a handler. It is intentionally close to `videoExts` in
  `app/player/playlist.go`, but they serve different jobs (mime types vs.
  sibling-scanning extensions) — update both when adding a format.
- `Exec=govi %f` (single local path, not `%F`/`%u`): govi takes at most one
  file argument, and a `file://` URI would break the folder playlist scan.
- `StartupWMClass=govi` must match the `appID` constant in
  `app/player/player.go`, which is fed to the GLFW `X11ClassName` /
  `X11InstanceName` hints. If they drift, the shell shows a generic icon for
  the running window. Verify with
  `xprop -id "$(xdotool search --classname govi | head -1)" WM_CLASS`.
- `StartupNotify=false` because GLFW does not consume `DESKTOP_STARTUP_ID`;
  with it enabled the shell shows a spinner until it times out.
- `zarf/postinstall.sh` / `postremove.sh` refresh the desktop, mime and
  icon caches (all guarded on the tool existing) so no re-login is needed.
- The tar.gz archive deliberately ships the binary only — desktop files
  belong to a system-wide install path the tarball does not own.
- The PNGs are rendered from the SVG:
  `for s in 32 48 64 128 256; do inkscape -w $s -h $s zarf/govi.svg -o zarf/govi-$s.png; done`
  (`rsvg-convert -w $s -h $s` works too). They are committed, so no build-time
  dependency on either tool.
- `zarf/govi.svg` is the flattened, Inkscape-metadata-free form of the artwork
  in `zarf/design/icon.svg` — the same play glyph as the idle-screen logo
  (`app/player/assets/logo.png`, exported from the same design file). Re-export
  both when the artwork changes so the launcher icon and the idle screen agree.

## License gate

`make license-check` (part of `verify`, and a standalone CI workflow) runs
go-licence-detector against `allowedLicenses.json` with
`overrideLicenses.json` for exceptions. A new dependency with an unlisted
license fails CI — update the rules deliberately, don't bypass. The project
itself is GPL-3.0.

## Local snapshot

`make build` = `goreleaser build --snapshot --clean --single-target`; output
under `dist/` (git-ignored, `make clean` removes it).
