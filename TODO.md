# TODO, unsorted of pending implementation items


- [] Make file browable similar to qimbv
- [] nice video controls
  - [] settings button
  - [x] volume button (mute toggle + volume slider, `app/player/volume.go`)
- [x] macOS `.app` bundle + `.dmg` — built by `zarf/macos` from a goreleaser
  post-build hook, installed by the cask's `app` stanza. See
  docs/agents/releasing.md.
- [] Verify on a Mac that double-clicking a video opens it in govi: the
  `application:openFiles:` handler (`app/player/openfiles_darwin.m`) compiles
  and links in CI, but nothing proves the Apple Event actually arrives. Also
  worth checking the Dock-icon drop path, which uses the same handler.
- [] Code-sign and notarize the macOS build, so the cask can drop the
  `xattr -dr com.apple.quarantine` hook. Needs a paid Apple Developer account.
- [] Make `make test` pass on macOS (headless mpv + Gio measurement tests).
- [] image viewer ?
  - [] selectable by the binary name, using symlincs
  - [] selectabe by shortcut / file type
- [] allow recursive videos with max amount configurable
