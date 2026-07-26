//go:build windows

package trash

import (
	"strings"
	"testing"
)

func TestDoubleNulTerminated(t *testing.T) {
	got, err := doubleNulTerminated(`C:\videos\movie.mkv`)
	if err != nil {
		t.Fatalf("doubleNulTerminated: %v", err)
	}

	// SHFileOperationW requires the pFrom list to end in two NULs: one closing
	// the last path, one closing the list. Getting this wrong makes the call
	// read past the buffer.
	if n := len(got); got[n-1] != 0 || got[n-2] != 0 {
		t.Errorf("buffer does not end in two NULs: %v", got[n-3:])
	}
	if got[len(got)-3] == 0 {
		t.Error("buffer ends in three NULs, want exactly two")
	}

	var b strings.Builder
	for _, u := range got[:len(got)-2] {
		b.WriteRune(rune(u))
	}
	if b.String() != `C:\videos\movie.mkv` {
		t.Errorf("path = %q, want %q", b.String(), `C:\videos\movie.mkv`)
	}
}

func TestDoubleNulTerminatedRejectsEmbeddedNul(t *testing.T) {
	// A NUL in the name would truncate the list and delete the wrong thing.
	if _, err := doubleNulTerminated("bad\x00name.mkv"); err == nil {
		t.Fatal("doubleNulTerminated with an embedded NUL returned nil, want error")
	}
}
