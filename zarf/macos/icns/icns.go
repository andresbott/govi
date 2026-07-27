// Package icns writes Apple .icns icon files from PNG images.
//
// The format needs no macOS tooling: an .icns file is the ASCII magic "icns",
// a big-endian total length, and then a sequence of entries, each of which is a
// 4-byte OSType, a big-endian length *including* the 8-byte entry header, and
// the payload. For the ic07..ic14 types the payload is a PNG file verbatim, so
// building one is concatenation plus two length fields — which is why govi does
// not need `iconutil` or `sips` (and therefore does not need a macOS host to
// produce its icon).
package icns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"sort"
)

// magic is the file's leading OSType.
const magic = "icns"

// headerLen is the size of both the file header (magic + length) and of each
// entry header (OSType + length). They happen to be the same 8 bytes.
const headerLen = 8

// sizeToType maps a square pixel size to the OSType that declares it.
//
// Only the PNG-carrying types are listed, and only one per size: several types
// describe the same pixel count at different logical scales (ic08 is 256px
// @1x, ic13 is 256px as 128@2x), and emitting both from a single source PNG
// would double the file for no visual gain. macOS picks the closest available
// size, so a sparse set is fine as long as it spans small and large.
var sizeToType = map[int]string{
	32:   "ic11", // 16x16@2x
	64:   "ic12", // 32x32@2x
	128:  "ic07",
	256:  "ic08",
	512:  "ic09",
	1024: "ic10", // 512x512@2x
}

// ErrNoImages is returned when Build is given nothing to encode.
var ErrNoImages = errors.New("icns: no images given")

// Build encodes the PNG files in pngs into a single .icns document.
//
// Each element is the raw content of a PNG file. Its pixel size is read from
// the PNG header rather than taken on trust, because the OSType has to match
// the real dimensions: macOS renders whatever size the type promises, so a
// mislabelled entry shows up as a blurred or clipped icon rather than as an
// error. Non-square images and sizes with no corresponding OSType are rejected
// for the same reason.
//
// Entries are written in ascending size order. The format does not require it,
// but a deterministic order makes the output byte-identical across runs, which
// keeps release artifacts reproducible.
func Build(pngs [][]byte) ([]byte, error) {
	if len(pngs) == 0 {
		return nil, ErrNoImages
	}

	type entry struct {
		size int
		typ  string
		data []byte
	}
	entries := make([]entry, 0, len(pngs))
	seen := make(map[int]bool, len(pngs))

	for i, data := range pngs {
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("icns: image %d: %w", i, err)
		}
		if cfg.Width != cfg.Height {
			return nil, fmt.Errorf("icns: image %d is %dx%d, icons must be square", i, cfg.Width, cfg.Height)
		}
		typ, ok := sizeToType[cfg.Width]
		if !ok {
			return nil, fmt.Errorf("icns: image %d has unsupported size %d, want one of %v", i, cfg.Width, Sizes())
		}
		if seen[cfg.Width] {
			return nil, fmt.Errorf("icns: duplicate image for size %d", cfg.Width)
		}
		seen[cfg.Width] = true
		entries = append(entries, entry{size: cfg.Width, typ: typ, data: data})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].size < entries[j].size })

	var body bytes.Buffer
	for _, e := range entries {
		// The length field counts the entry header as well as the payload, so a
		// reader can walk the file by adding it to the entry's own offset.
		if _, err := body.WriteString(e.typ); err != nil {
			return nil, err
		}
		n, err := lengthField(headerLen + len(e.data))
		if err != nil {
			return nil, fmt.Errorf("icns: %s entry: %w", e.typ, err)
		}
		if err := binary.Write(&body, binary.BigEndian, n); err != nil {
			return nil, err
		}
		if _, err := body.Write(e.data); err != nil {
			return nil, err
		}
	}

	total, err := lengthField(headerLen + body.Len())
	if err != nil {
		return nil, fmt.Errorf("icns: document: %w", err)
	}
	var out bytes.Buffer
	out.WriteString(magic)
	if err := binary.Write(&out, binary.BigEndian, total); err != nil {
		return nil, err
	}
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

// lengthField converts a byte count to the format's 32-bit big-endian length.
//
// The bound is the format's, not a defensive guess: both length fields are
// uint32, so anything at or above 4 GiB cannot be represented and would wrap to
// a small value — producing a file that parses as valid and is silently
// truncated. Real icons are ~200 KB, so this only ever fires on a caller
// mistake.
func lengthField(n int) (uint32, error) {
	if n < 0 || int64(n) > math.MaxUint32 {
		return 0, fmt.Errorf("length %d does not fit the format's 32-bit field", n)
	}
	return uint32(n), nil
}

// Sizes returns the pixel sizes Build accepts, ascending. Used in error
// messages and by callers deciding which PNGs to render.
func Sizes() []int {
	out := make([]int, 0, len(sizeToType))
	for s := range sizeToType {
		out = append(out, s)
	}
	sort.Ints(out)
	return out
}

// Encode is a convenience wrapper that PNG-encodes each image before building.
// It exists for callers holding decoded images rather than PNG files.
func Encode(imgs []image.Image) ([]byte, error) {
	pngs := make([][]byte, 0, len(imgs))
	for i, img := range imgs {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("icns: encode image %d: %w", i, err)
		}
		pngs = append(pngs, buf.Bytes())
	}
	return Build(pngs)
}
