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

func TestDecodePNGRejectsGarbage(t *testing.T) {
	if _, err := decodePNG([]byte("not a png")); err == nil {
		t.Fatal("expected decode error for garbage bytes")
	}
}
