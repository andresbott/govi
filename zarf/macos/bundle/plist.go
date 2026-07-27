package bundle

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// plistValue is one node of a property list. Only the four types an Info.plist
// needs are modelled — writing the XML through a tiny typed tree rather than a
// text template is what keeps every string escaped (a version or path with an
// `&` in it would otherwise produce a plist macOS refuses to parse, and
// Launch Services fails such a bundle silently).
type plistValue interface {
	writeTo(buf *bytes.Buffer, indent string) error
}

type plistString string

func (s plistString) writeTo(buf *bytes.Buffer, indent string) error {
	buf.WriteString(indent)
	buf.WriteString("<string>")
	if err := xml.EscapeText(buf, []byte(s)); err != nil {
		return err
	}
	buf.WriteString("</string>\n")
	return nil
}

type plistBool bool

func (b plistBool) writeTo(buf *bytes.Buffer, indent string) error {
	buf.WriteString(indent)
	if b {
		buf.WriteString("<true/>\n")
	} else {
		buf.WriteString("<false/>\n")
	}
	return nil
}

type plistArray []plistValue

func (a plistArray) writeTo(buf *bytes.Buffer, indent string) error {
	buf.WriteString(indent + "<array>\n")
	for _, v := range a {
		if err := v.writeTo(buf, indent+"\t"); err != nil {
			return err
		}
	}
	buf.WriteString(indent + "</array>\n")
	return nil
}

// plistDict keeps its own key order rather than relying on a Go map, because a
// plist is compared byte-for-byte across releases (reproducible artifacts) and
// map iteration would reorder it on every run.
type plistDict struct {
	keys   []string
	values map[string]plistValue
}

func newDict() *plistDict {
	return &plistDict{values: map[string]plistValue{}}
}

func (d *plistDict) set(key string, v plistValue) {
	if _, ok := d.values[key]; !ok {
		d.keys = append(d.keys, key)
	}
	d.values[key] = v
}

func (d *plistDict) writeTo(buf *bytes.Buffer, indent string) error {
	buf.WriteString(indent + "<dict>\n")
	for _, k := range d.keys {
		buf.WriteString(indent + "\t<key>")
		if err := xml.EscapeText(buf, []byte(k)); err != nil {
			return err
		}
		buf.WriteString("</key>\n")
		if err := d.values[k].writeTo(buf, indent+"\t"); err != nil {
			return err
		}
	}
	buf.WriteString(indent + "</dict>\n")
	return nil
}

// Info is the metadata that goes into Contents/Info.plist.
type Info struct {
	// Name is the user-visible app name (menu bar, Finder, Launchpad).
	Name string
	// Identifier is the reverse-DNS bundle id. macOS keys per-app state
	// (Launch Services registration, window position, permissions) on it, so it
	// must stay stable across releases.
	Identifier string
	// Version is the release version as goreleaser reports it, without a
	// leading "v" — e.g. "0.1.5" or "0.1.5-rc1".
	Version string
	// Executable is the binary's name inside Contents/MacOS.
	Executable string
	// Icon is the icon file's name inside Contents/Resources, with or without
	// the .icns extension.
	Icon string
	// MinSystem is the LSMinimumSystemVersion value.
	MinSystem string
	// VideoExtensions makes the app a handler for these file extensions (given
	// without a dot). Empty means the bundle declares no document types, so
	// govi never appears in Finder's "Open with" list.
	VideoExtensions []string
}

// numericVersion matches the leading dotted-numeric part of a version string.
var numericVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*`)

// bundleVersion reduces a release version to the dotted-numeric form
// CFBundleVersion is specified to hold.
//
// This matters for upgrades, not cosmetics: Homebrew compares the installed and
// incoming bundle versions when deciding whether a cask upgrade is a no-op, and
// macOS itself parses the field. A prerelease like "0.1.5-rc1" is not a legal
// value, so the suffix is dropped here and kept in
// CFBundleShortVersionString, which is the free-form display string.
//
// A version with no numeric prefix at all (goreleaser's "dev-build" default in
// a snapshot) falls back to "0", since the key is required.
func bundleVersion(v string) string {
	if m := numericVersion.FindString(strings.TrimPrefix(v, "v")); m != "" {
		return m
	}
	return "0"
}

// shortVersion is the display version: the full string, minus a leading "v".
func shortVersion(v string) string {
	if v = strings.TrimPrefix(v, "v"); v != "" {
		return v
	}
	return "0"
}

// InfoPlist renders info as an Info.plist document.
func InfoPlist(info Info) ([]byte, error) {
	if info.Name == "" {
		return nil, fmt.Errorf("bundle: Info.Name is required")
	}
	if info.Identifier == "" {
		return nil, fmt.Errorf("bundle: Info.Identifier is required")
	}
	if info.Executable == "" {
		return nil, fmt.Errorf("bundle: Info.Executable is required")
	}

	d := newDict()
	d.set("CFBundleName", plistString(info.Name))
	d.set("CFBundleDisplayName", plistString(info.Name))
	d.set("CFBundleIdentifier", plistString(info.Identifier))
	d.set("CFBundleExecutable", plistString(info.Executable))
	d.set("CFBundleShortVersionString", plistString(shortVersion(info.Version)))
	d.set("CFBundleVersion", plistString(bundleVersion(info.Version)))
	d.set("CFBundlePackageType", plistString("APPL"))
	// The four-question-mark signature is the documented "none" value; govi has
	// no registered creator code.
	d.set("CFBundleSignature", plistString("????"))
	d.set("CFBundleInfoDictionaryVersion", plistString("6.0"))
	if info.Icon != "" {
		d.set("CFBundleIconFile", plistString(strings.TrimSuffix(info.Icon, ".icns")))
	}
	if info.MinSystem != "" {
		d.set("LSMinimumSystemVersion", plistString(info.MinSystem))
	}
	// Without NSPrincipalClass the process is treated as a background tool: no
	// menu bar and no Dock presence, which would defeat the whole bundle.
	d.set("NSPrincipalClass", plistString("NSApplication"))
	// govi renders at the framebuffer's real resolution (see the darwin content
	// -scale handling in app/player/input.go); without this macOS would upscale
	// a 1x surface and the video would look soft on a Retina display.
	d.set("NSHighResolutionCapable", plistBool(true))
	// govi is a normal windowed app, not an agent.
	d.set("LSUIElement", plistBool(false))

	if len(info.VideoExtensions) > 0 {
		d.set("CFBundleDocumentTypes", documentTypes(info.Name, info.VideoExtensions))
	}

	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	body.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	body.WriteString(`<plist version="1.0">` + "\n")
	if err := d.writeTo(&body, ""); err != nil {
		return nil, err
	}
	body.WriteString("</plist>\n")
	return body.Bytes(), nil
}

// documentTypes builds the CFBundleDocumentTypes array: what makes govi appear
// in Finder's "Open with" menu and be selectable as the default handler.
//
// Two entries rather than one, because the two matching mechanisms cover
// different gaps. `public.movie` is the UTI supertype most video formats
// conform to, and is what modern macOS matches on — but formats without a
// system-declared UTI (.mkv, .webm, .ogv among them) conform to nothing and
// would never match it. The extension entry catches exactly those.
//
// Both are declared with LSHandlerRank "Alternate": govi offers itself as a
// choice without claiming to be the system's preferred player for every video
// on the disk. The user still picks it via "Open with" or sets it as default.
func documentTypes(name string, exts []string) plistArray {
	uti := newDict()
	uti.set("CFBundleTypeName", plistString(name+" video"))
	uti.set("CFBundleTypeRole", plistString("Viewer"))
	uti.set("LSHandlerRank", plistString("Alternate"))
	uti.set("LSItemContentTypes", plistArray{
		plistString("public.movie"),
		plistString("public.video"),
		plistString("public.audiovisual-content"),
	})

	sorted := make([]string, len(exts))
	copy(sorted, exts)
	for i, e := range sorted {
		sorted[i] = strings.TrimPrefix(strings.ToLower(e), ".")
	}
	sort.Strings(sorted)
	values := make(plistArray, 0, len(sorted))
	for _, e := range sorted {
		values = append(values, plistString(e))
	}

	byExt := newDict()
	byExt.set("CFBundleTypeName", plistString(name+" video file"))
	byExt.set("CFBundleTypeRole", plistString("Viewer"))
	byExt.set("LSHandlerRank", plistString("Alternate"))
	byExt.set("CFBundleTypeExtensions", values)

	return plistArray{uti, byExt}
}
