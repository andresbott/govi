//go:build !windows && !darwin

package trash

import "testing"

func TestEscapePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "/home/u/a.mkv", "/home/u/a.mkv"},
		{"slashes kept", "/a/b/c", "/a/b/c"},
		{"space", "/home/u/my file.mkv", "/home/u/my%20file.mkv"},
		{"percent", "/home/u/100%.mkv", "/home/u/100%25.mkv"},
		{"newline", "/home/u/a\nb", "/home/u/a%0Ab"},
		{"hash and question", "/home/u/a#b?c", "/home/u/a%23b%3Fc"},
		{"utf8 escaped", "/home/u/ñ.mkv", "/home/u/%C3%B1.mkv"},
		{"unreserved kept", "/home/u/a-b_c.d~e", "/home/u/a-b_c.d~e"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapePath(tt.in); got != tt.want {
				t.Errorf("escapePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUniqueName(t *testing.T) {
	tests := []struct {
		name string
		base string
		id   int
		want string
	}{
		{"first attempt unchanged", "movie.mkv", 1, "movie.mkv"},
		{"suffix before extension", "movie.mkv", 2, "movie.2.mkv"},
		{"no extension", "movie", 3, "movie.3"},
		{"first dot wins", "a.tar.gz", 2, "a.2.tar.gz"},
		{"dotfile", ".hidden", 2, ".2.hidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uniqueName(tt.base, tt.id); got != tt.want {
				t.Errorf("uniqueName(%q, %d) = %q, want %q", tt.base, tt.id, got, tt.want)
			}
		})
	}
}
