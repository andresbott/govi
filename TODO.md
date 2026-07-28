# TODO, unsorted of pending implementation items


- [] Make file browable similar to qimbv
- [] nice video controls
  - [] settings button
  - [x] volume button (mute toggle + volume slider, `app/player/volume.go`)
- [x] macOS `.app` bundle + `.dmg` — built by `zarf/macos` from a goreleaser
  post-build hook, installed by the cask's `app` stanza. See
  docs/agents/releasing.md.
- [] Verify on a Mac that Finder double-click and Dock-icon drop open a video in
  govi. Both go through `application:openFiles:`
  (`app/player/openfiles_darwin.m`), which compiles and links in CI, but nothing
  proves the Apple Event actually arrives — no test can reach the file (it is
  Objective-C and macOS-only, and `openfiles_test.go` is tagged
  `!darwin || !cgo`), so this is a manual check. What *has* been exercised on a
  Mac is `govi <video>` from a shell on a bundled build: that is where the
  argv-echo open event was found (see the `goviLaunching` comment there and
  docs/agents/releasing.md).
- [] Code-sign and notarize the macOS build, so the cask can drop the
  `xattr -dr com.apple.quarantine` hook. Needs a paid Apple Developer account.
- [] Make `make test` pass on macOS (headless mpv + Gio measurement tests).
- [] image viewer ?
  - [] selectable by the binary name, using symlincs
  - [] selectabe by shortcut / file type
- [] allow recursive videos with max amount configurable
- [] dont show the osd on KB shotcuts
