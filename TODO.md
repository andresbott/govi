# TODO, unsorted of pending implementation items


- [] Make file browable similar to qimbv
- [] nice video controls
  - [] settings button
  - [x] volume button (mute toggle + volume slider, `app/player/volume.go`)
- [] macOS `.app` bundle — a Dock icon, Launchpad entry and Finder "Open with →
  govi" need a `govi.app` with an `Info.plist` declaring `CFBundleDocumentTypes`
  and a `.icns` rendered from `zarf/govi.svg`. GoReleaser's `app_bundles`/`dmg`
  sections generate exactly this but are Pro-only, so it means hand-writing the
  bundle plus the cask's `app` stanza (losing automatic cask regeneration) or
  buying Pro. The `.deb` already has the equivalent via `zarf/govi.desktop`.
- [] macOS code signing + notarization — would remove the `xattr` quarantine
  hack from the cask; needs a paid Apple Developer account.
- [] Intel Mac builds — a `macos-*-intel` runner job and an `on_intel` cask
  block (billed at 12x the arm64 runner rate).
- [] Make `make test` pass on macOS (headless mpv + Gio measurement tests).