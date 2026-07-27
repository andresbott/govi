# govi

A minimalistic video player based on mpv (libmpv via go-mpv): it plays a file,
gets out of the way, and leaves the keyboard in charge.

> **Status:** under development. Things work, but interfaces, config and
> shortcuts may still change between versions.

## Screenshots

![govi playing a video](zarf/MainScreen.jpg)

![govi media info overlay](zarf/infoScreen.jpg)

## Install

### macOS (Homebrew)

A macOS cask is published into this repository on every tagged release. Because the repo
isn't named `homebrew-*`, tap it with an explicit URL, then install:

```bash
brew tap andresbott/govi https://github.com/andresbott/govi
brew install --cask andresbott/govi/govi
```

`mpv` is pulled in as a dependency (it provides libmpv), and `brew upgrade` will track
future releases. Apple Silicon only.

This installs the `govi` **terminal command** — there is no Dock icon and no
"Open with → govi" in Finder yet; run `govi <file>` from a terminal.

### Debian / Ubuntu

Download the `.deb` from the
[releases page](https://github.com/andresbott/govi/releases) and install it
(this also pulls in libmpv):

```bash
sudo apt install ./govi_*_amd64.deb
```

### Other

Grab a prebuilt `tar.gz` archive from the
[releases page](https://github.com/andresbott/govi/releases).

## Usage

```sh
govi <file>    # play a file
govi           # open the idle screen (drop a file on it)
govi version
```

## Distinctive features

- **Automatic playlist** — opening a file makes its folder the playlist, so
  next/previous walks the sibling videos without any setup.
- **File deletion from the player** — move to trash or delete permanently,
  with a confirmation step and auto-advance to the next video.
- **Fully controllable with shortcuts** — every action is keyboard-driven and
  rebindable, either from the preferences overlay or the config file.

## Development

Common tasks are driven by make:

```sh
make help     # list all targets
make test     # run the go tests
make lint     # run golangci-lint
make verify   # full verification suite (test, lint, license-check, benchmark, coverage)
make build    # build a snapshot binary with goreleaser
```

### Release

Releases are published by GitHub Actions when a version tag is pushed:

```sh
make tag version="v0.1.0"
```
