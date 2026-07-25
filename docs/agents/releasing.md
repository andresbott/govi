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

## License gate

`make license-check` (part of `verify`, and a standalone CI workflow) runs
go-licence-detector against `allowedLicenses.json` with
`overrideLicenses.json` for exceptions. A new dependency with an unlisted
license fails CI — update the rules deliberately, don't bypass. The project
itself is GPL-3.0.

## Local snapshot

`make build` = `goreleaser build --snapshot --clean --single-target`; output
under `dist/` (git-ignored, `make clean` removes it).
