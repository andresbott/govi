# govi

Minimalistic video player based on mpv (libmpv via go-mpv).

## Status

Early bootstrap: only the CLI skeleton with a `version` command exists so far.

## Usage

```sh
govi version
```

## Development

Common tasks are driven by make:

```sh
make help     # list all targets
make test     # run the go tests
make lint     # run golangci-lint
make verify   # full verification suite (test, lint, license-check, benchmark, coverage)
make build    # build a snapshot binary with goreleaser
```

## Release

Releases are published by GitHub Actions when a version tag is pushed:

```sh
make tag version="v0.1.0"
```

## License

GPL-3.0, see [LICENSE](LICENSE).
