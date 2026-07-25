# libs/trash

Move files and directories to the desktop trash, using each OS's native
mechanism so the result is what the user's file manager shows and can restore.

```go
if err := trash.Move("/path/to/file.mkv"); err != nil {
	// the item is still at its original location
}
```

`Move` returns nil only when the item is really gone from its original path.
A missing path gives an error satisfying `os.IsNotExist`; an empty one gives
`ErrEmptyPath`.

## Per-platform behaviour

| OS | Mechanism | Notes |
|---|---|---|
| Windows | `SHFileOperationW` (`FO_DELETE` + `FOF_ALLOWUNDO`) | The shell picks the correct per-volume `$Recycle.Bin`. Restorable from Explorer. No dialogs. |
| macOS | `-[NSFileManager trashItemAtURL:]` via cgo | Picks `~/.Trash` or the volume's `.Trashes/<uid>`, and writes the metadata Finder's **Put Back** needs. Requires cgo — see below. |
| Linux / BSD | FreeDesktop.org Trash spec | Home trash under `$XDG_DATA_HOME/Trash`, or a per-volume trash on the item's own filesystem. Equivalent to `gio trash`. |

## Two design rules

**Never copy.** If an item cannot be moved into a trash directory on its own
filesystem, `Move` fails. It does not fall back to copy-then-delete. Trashing a
4 GB video from a Samba share must not pull 4 GB across the network into
`$HOME` — so on Linux the item goes to `.Trash-$uid` at the top of that mount
instead. Because every successful trash is a metadata-only rename, `Move` is
cheap enough to call from a UI thread.

**Never report a false success.** A nil error means the item is gone. This is
why `Move` stats the path on Windows and macOS before handing off: both APIs
report success for an item that was never there.

## Linux details

Resolution order for the trash directory, per the spec:

1. Same filesystem as `$XDG_DATA_HOME` → the home trash, `Path=` absolute.
2. Otherwise, `$topdir/.Trash/$uid` if `$topdir/.Trash` exists, is a real
   directory, and is sticky — then `$topdir/.Trash-$uid`. `Path=` is recorded
   relative to `$topdir` so the entry survives the volume being remounted
   elsewhere.

The same-filesystem check compares the *parent* directory's `st_dev` for
non-directories: on overlayfs a file's device differs from its directory's, and
it is the directory that decides whether a rename can work.

Directories are created `0700` (the spec's recommendation — trashed files keep
their original contents and must not become world-readable), name collisions
resolve as `movie.mkv` → `movie.2.mkv`, and a failed rename removes the
`.trashinfo` it had reserved rather than leaving it orphaned.

## Verification status

- **Linux** — fully tested, including the cross-filesystem path (the suite finds
  a second filesystem such as `/dev/shm`, and skips those cases if none exists).
- **Windows** — the path-marshalling logic is unit-tested and the package
  compiles for `windows/amd64` and `windows/386`, but the `SHFileOperationW`
  call itself has **not been executed**; it needs a Windows runner.
- **macOS** — **not compiled or executed.** The cgo file needs clang and the
  macOS SDK. The Objective-C was written against Apple's documented signature
  for `trashItemAtURL:resultingItemURL:error:` (macOS 10.8+), but it needs a
  real build to be trusted.

A `darwin && !cgo` build returns a clear "requires a cgo build" error rather
than failing to link. govi always builds with cgo (`go-gl/glfw` requires it), so
that path is a safety net, not a supported mode.
