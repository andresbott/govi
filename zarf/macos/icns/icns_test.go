package icns

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// pngOfSize returns the bytes of a w x h PNG, for feeding Build.
func pngOfSize(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode %dx%d png: %v", w, h, err)
	}
	return buf.Bytes()
}

// parse walks an .icns document the way a reader would: it follows the entry
// lengths rather than assuming where the payloads are, so a wrong length field
// shows up as a parse failure here.
func parse(t *testing.T, doc []byte) (total uint32, types []string, payloads [][]byte) {
	t.Helper()
	if len(doc) < headerLen {
		t.Fatalf("document is %d bytes, shorter than the header", len(doc))
	}
	if got := string(doc[:4]); got != magic {
		t.Fatalf("magic = %q, want %q", got, magic)
	}
	total = binary.BigEndian.Uint32(doc[4:8])
	for off := headerLen; off < len(doc); {
		if off+headerLen > len(doc) {
			t.Fatalf("entry header at %d runs past the end", off)
		}
		typ := string(doc[off : off+4])
		n := binary.BigEndian.Uint32(doc[off+4 : off+8])
		if n < headerLen || off+int(n) > len(doc) {
			t.Fatalf("entry %q at %d has length %d, which does not fit", typ, off, n)
		}
		types = append(types, typ)
		payloads = append(payloads, doc[off+headerLen:off+int(n)])
		off += int(n)
	}
	return total, types, payloads
}

func TestBuildLayout(t *testing.T) {
	tests := []struct {
		name      string
		sizes     []int
		wantTypes []string
	}{
		{
			name:      "single size",
			sizes:     []int{256},
			wantTypes: []string{"ic08"},
		},
		{
			name:      "sorted ascending regardless of input order",
			sizes:     []int{512, 32, 128},
			wantTypes: []string{"ic11", "ic07", "ic09"},
		},
		{
			name:      "every supported size",
			sizes:     []int{32, 64, 128, 256, 512, 1024},
			wantTypes: []string{"ic11", "ic12", "ic07", "ic08", "ic09", "ic10"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var pngs [][]byte
			for _, s := range tc.sizes {
				pngs = append(pngs, pngOfSize(t, s, s))
			}

			doc, err := Build(pngs)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			total, types, payloads := parse(t, doc)

			// The header length must describe the whole file: a reader trusting it
			// would otherwise stop early or run off the end.
			if int(total) != len(doc) {
				t.Errorf("header length = %d, want %d (the document size)", total, len(doc))
			}
			if len(types) != len(tc.wantTypes) {
				t.Fatalf("got %d entries (%v), want %d", len(types), types, len(tc.wantTypes))
			}
			for i, want := range tc.wantTypes {
				if types[i] != want {
					t.Errorf("entry %d type = %q, want %q", i, types[i], want)
				}
			}

			// Payloads must come back byte-identical: macOS decodes them as PNG
			// files, so any framing added around them would break the icon.
			for i, got := range payloads {
				if _, err := png.DecodeConfig(bytes.NewReader(got)); err != nil {
					t.Errorf("entry %d payload is not a decodable PNG: %v", i, err)
				}
			}
		})
	}
}

// The OSType has to match the payload's real pixel size, because macOS renders
// at the size the type promises. This pins that the mapping is read from the
// image rather than from the caller's ordering.
func TestBuildTypeMatchesImageSize(t *testing.T) {
	for _, size := range Sizes() {
		doc, err := Build([][]byte{pngOfSize(t, size, size)})
		if err != nil {
			t.Fatalf("Build(%d): %v", size, err)
		}
		_, types, payloads := parse(t, doc)
		if len(types) != 1 {
			t.Fatalf("size %d: got %d entries, want 1", size, len(types))
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(payloads[0]))
		if err != nil {
			t.Fatalf("size %d: decode payload: %v", size, err)
		}
		if cfg.Width != size {
			t.Errorf("type %s carries a %dpx image, want %dpx", types[0], cfg.Width, size)
		}
	}
}

func TestBuildRejects(t *testing.T) {
	tests := []struct {
		name string
		pngs [][]byte
	}{
		{
			name: "no images",
			pngs: nil,
		},
		{
			name: "not a png",
			pngs: [][]byte{[]byte("this is not a png")},
		},
		{
			name: "non-square",
			pngs: [][]byte{pngOfSize(t, 256, 128)},
		},
		{
			name: "size with no ostype",
			pngs: [][]byte{pngOfSize(t, 100, 100)},
		},
		{
			name: "duplicate size",
			pngs: [][]byte{pngOfSize(t, 256, 256), pngOfSize(t, 256, 256)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Build(tc.pngs); err == nil {
				t.Fatal("Build succeeded, want an error")
			}
		})
	}
}

// Release artifacts should be reproducible, so the same inputs must give the
// same bytes — including when the caller's slice order differs.
func TestBuildIsDeterministic(t *testing.T) {
	a := [][]byte{pngOfSize(t, 128, 128), pngOfSize(t, 512, 512), pngOfSize(t, 32, 32)}
	b := [][]byte{pngOfSize(t, 32, 32), pngOfSize(t, 128, 128), pngOfSize(t, 512, 512)}

	first, err := Build(a)
	if err != nil {
		t.Fatalf("Build(a): %v", err)
	}
	second, err := Build(b)
	if err != nil {
		t.Fatalf("Build(b): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("Build is order-dependent: the same images in a different order gave different bytes")
	}
}

func TestEncode(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	doc, err := Encode([]image.Image{img})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, types, _ := parse(t, doc)
	if len(types) != 1 || types[0] != "ic08" {
		t.Errorf("types = %v, want [ic08]", types)
	}
}
