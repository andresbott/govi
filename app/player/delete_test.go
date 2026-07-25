package player

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemovalSucceeded(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "still-here.mkv")
	if err := os.WriteFile(present, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	gone := filepath.Join(dir, "gone.mkv")

	tests := []struct {
		name string
		path string
		err  error
		want bool
	}{
		{"trashed cleanly", gone, nil, true},
		{"error and file gone anyway", gone, errors.New("boom"), false},
		// The load-bearing case: a trash backend that reports success while
		// leaving the file in place must not advance the playlist, or the user
		// sees the file skipped and believes it was trashed.
		{"reported success but file remains", present, nil, false},
		{"error and file remains", present, errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removalSucceeded(tt.path, tt.err); got != tt.want {
				t.Errorf("removalSucceeded(%q, %v) = %v, want %v", tt.path, tt.err, got, tt.want)
			}
		})
	}
}
