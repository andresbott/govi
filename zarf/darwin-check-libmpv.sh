#!/bin/sh
# Assert that the darwin binary loads libmpv through Homebrew's *opt* path
# (/opt/homebrew/opt/mpv/lib/libmpv.N.dylib) rather than a versioned Cellar
# path. A cgo binary hardcodes the dylib path it was linked against — macOS
# dyld does no library search — so a Cellar-pinned reference would stop
# resolving the moment the user runs "brew upgrade mpv".
#
# Homebrew already rewrites install names to the opt path before bottling, so
# this is a guard against that changing, not a fix. Deliberately read-only:
# rewriting with install_name_tool would invalidate the ad-hoc code signature,
# which Apple Silicon requires, forcing a re-sign step.
set -eu

if [ $# -ne 1 ]; then
	echo "usage: $0 <path-to-binary>" >&2
	exit 2
fi
bin=$1

if [ ! -f "$bin" ]; then
	echo "error: no such binary: $bin" >&2
	exit 2
fi

# OTOOL is overridable so this can be tested off macOS with a stub.
OTOOL=${OTOOL:-otool}

linkage=$("$OTOOL" -L "$bin")

if printf '%s\n' "$linkage" | grep -q '/Cellar/mpv/'; then
	echo "error: govi is linked against a versioned Cellar path for libmpv." >&2
	echo "It would break as soon as the user runs 'brew upgrade mpv'." >&2
	printf '%s\n' "$linkage" >&2
	exit 1
fi

if ! printf '%s\n' "$linkage" | grep -q '/opt/mpv/lib/libmpv\.'; then
	echo "error: govi has no libmpv reference under an mpv opt path." >&2
	printf '%s\n' "$linkage" >&2
	exit 1
fi

echo "ok: libmpv is linked through the upgrade-stable opt path"
