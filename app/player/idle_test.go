package player

import (
	"log/slog"
	"testing"
)

func TestLoadPlaceholderEmbeddedWhenNoPath(t *testing.T) {
	img := loadPlaceholder("", slog.Default())
	if img == nil {
		t.Fatal("expected embedded placeholder, got nil")
	}
	if img.Bounds().Dx() == 0 {
		t.Fatal("embedded placeholder has zero width")
	}
}

func TestLoadPlaceholderFallsBackOnBadPath(t *testing.T) {
	img := loadPlaceholder("/no/such/file.png", slog.Default())
	if img == nil {
		t.Fatal("expected fallback to embedded image, got nil")
	}
}

func TestLoadLogoDecodesEmbedded(t *testing.T) {
	img := loadLogo(slog.Default())
	if img == nil {
		t.Fatal("expected embedded logo, got nil")
	}
	// A 1x1 image is the decode-failure fallback, so this also catches a
	// broken or truncated asset, not just a missing one.
	if b := img.Bounds(); b.Dx() < 2 || b.Dy() < 2 {
		t.Fatalf("logo bounds = %v, want the real asset", b)
	}
}

func TestDecodeImageRejectsGarbage(t *testing.T) {
	if _, err := decodeImage([]byte("not an image")); err == nil {
		t.Fatal("expected decode error for garbage bytes")
	}
}
