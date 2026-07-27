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

## Build constraints

- **Two independent goreleaser runs, one release.** govi links libmpv via cgo, so
  a darwin binary must be compiled on macOS, and goreleaser OSS has no
  `--split` / `continue --merge` (Pro-only). So `.goreleaser.yaml` builds
  **linux amd64 only, CGO_ENABLED=1**, and `.goreleaser.darwin.yaml` builds
  **darwin arm64** on a `macos-latest` runner. Adding a platform means adding a
  native build runner, not a goarch line. Validate both with
  `make release-check`.
- Version info is linker-stamped into `app/metainfo` (`Version`, `BuildTime`,
  `ShaVer`); dev builds show `dev-build` and stamp `BuildTime` at init.
- Artifacts: tar.gz archive (uname-compatible naming) plus nfpm packages.

## The darwin run (`.goreleaser.darwin.yaml`)

`.github/workflows/release.yml` has two jobs. `release` (ubuntu) creates the
GitHub release and its changelog; `release-darwin` (`macos-latest`, Apple
Silicon, free for public repos) then appends the darwin artifacts. The
`needs: release` edge is load-bearing — the release and its notes must exist
before the second run touches it, which is also why the darwin config sets
`release.mode: keep-existing` and `changelog.disable: true`.

Four things the darwin config must keep:

- **`tags: [pkgconfig]`** — go-mpv's default cgo path is a bare `-lmpv`, and
  clang doesn't search `/opt/homebrew/{include,lib}`. The tag switches it to
  `pkg-config: mpv`; Homebrew's mpv formula ships `lib/pkgconfig/mpv.pc`
  (built with `-Dlibmpv=true`). The workflow installs `pkgconf` explicitly
  because of this.
- **`checksums_darwin.txt`** — two runs uploading to one release must not
  produce identically named assets, and the archive template already
  disambiguates by OS but the checksum file does not.
- **arm64 only** — Intel needs a `macos-*-intel` runner, billed at 12x the
  arm64 rate per release. The generated cask has an `on_arm` block only, so
  `brew install --cask` on an Intel Mac fails with an unsupported-architecture
  error rather than installing an unrunnable binary.
- **The quarantine hook** — the binary is unsigned and un-notarized, so
  without `xattr -dr com.apple.quarantine` Gatekeeper reports "govi is
  damaged and can't be opened". Removing this hack needs a paid Apple
  Developer account and a notarization step.

**Never add `--single-target` to the darwin build.** It matches the *host*
target, so on a mismatched runner goreleaser exits 0 having produced no binary
at all. Both workflows omit it and assert a binary exists afterwards.

**goreleaser is not preinstalled on the macOS runner images** (unlike ubuntu's),
so both darwin jobs invoke it through `goreleaser/goreleaser-action@v6` rather
than as a bare `run:` command — a plain `run: goreleaser ...` fails with
`command not found` (exit 127).

`zarf/darwin-check-libmpv.sh` runs as a build post-hook and **asserts** that
libmpv is loaded via Homebrew's opt path
(`/opt/homebrew/opt/mpv/lib/libmpv.2.dylib`), failing the build otherwise. It
does not rewrite anything: Homebrew's `fix_dynamic_linkage` already sets
install names from `opt_record` rather than the versioned Cellar path, so a
`brew upgrade mpv` doesn't break govi. Using `install_name_tool` here would
invalidate the ad-hoc code signature Apple Silicon requires. The script's
logic is tested on Linux via a stubbed `otool`
(`zarf/darwin_check_libmpv_test.go`, run by `make test`).

Two failure modes are accepted because no build-time fix addresses them: a
libmpv soname bump (needs a rebuild and a new tag) and a non-standard
`HOMEBREW_PREFIX` (`/opt/homebrew` is the only supported prefix on Apple
Silicon).

**What is and isn't verified.** CI proves govi *compiles and cgo-links* on
macOS arm64 (`build-darwin` in `test.yml` runs on every PR) and that the
linkage is opt-path based. It does **not** prove govi runs: the macOS test
suite is not green (headless mpv and Gio measurement tests need a
display/font environment), and nobody has launched the binary on a Mac. Treat
the first tagged release as the real test.

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
