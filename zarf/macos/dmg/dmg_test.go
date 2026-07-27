package dmg

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeBundle writes a minimal .app tree and returns its path.
func fakeBundle(t *testing.T, dir string) string {
	t.Helper()
	app := filepath.Join(dir, "govi.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents/MacOS"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents/Info.plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents/MacOS/govi"), []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	return app
}

func TestStage(t *testing.T) {
	tmp := t.TempDir()
	app := fakeBundle(t, tmp)
	stage := filepath.Join(tmp, "stage")

	got, err := Stage(Options{AppDir: app, StageDir: stage})
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if got != stage {
		t.Errorf("Stage returned %q, want %q", got, stage)
	}

	// The whole bundle must be present, with its modes intact: hdiutil copies
	// what it finds, so a binary that lost its x bit here ships broken.
	bin := filepath.Join(stage, "govi.app/Contents/MacOS/govi")
	st, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("staged binary: %v", err)
	}
	if st.Mode().Perm()&0o111 == 0 {
		t.Errorf("staged binary mode = %v, want the x bit set", st.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(stage, "govi.app/Contents/Info.plist")); err != nil {
		t.Errorf("staged Info.plist: %v", err)
	}
}

// The /Applications symlink is the drag-to-install affordance. It must be a
// symlink pointing at the absolute path — a copied directory would put a real
// (and empty) Applications folder in the image.
func TestStageCreatesApplicationsSymlink(t *testing.T) {
	tmp := t.TempDir()
	app := fakeBundle(t, tmp)
	stage := filepath.Join(tmp, "stage")

	if _, err := Stage(Options{AppDir: app, StageDir: stage}); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	link := filepath.Join(stage, "Applications")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat Applications: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Applications is not a symlink")
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "/Applications" {
		t.Errorf("Applications -> %q, want /Applications", target)
	}
}

func TestStageRejects(t *testing.T) {
	tmp := t.TempDir()
	app := fakeBundle(t, tmp)

	notEmpty := filepath.Join(tmp, "used")
	if err := os.MkdirAll(notEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notEmpty, "leftover"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(tmp, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		opts Options
	}{
		{name: "no app dir", opts: Options{StageDir: filepath.Join(tmp, "s1")}},
		{name: "no stage dir", opts: Options{AppDir: app}},
		{name: "missing bundle", opts: Options{AppDir: filepath.Join(tmp, "nope.app"), StageDir: filepath.Join(tmp, "s2")}},
		{name: "bundle is a file", opts: Options{AppDir: file, StageDir: filepath.Join(tmp, "s3")}},
		{name: "stage dir not empty", opts: Options{AppDir: app, StageDir: notEmpty}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Stage(tc.opts); err == nil {
				t.Fatal("Stage succeeded, want an error")
			}
		})
	}
}

// Off macOS, Create must fail with a clear reason rather than reporting success
// after producing nothing, or dying in exec with "hdiutil: not found".
func TestCreateOffDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("this asserts the non-darwin path")
	}
	tmp := t.TempDir()
	app := fakeBundle(t, tmp)
	_, err := Create(Options{
		AppDir:   app,
		Out:      filepath.Join(tmp, "govi.dmg"),
		StageDir: filepath.Join(tmp, "stage"),
	})
	if !errors.Is(err, ErrNotDarwin) {
		t.Fatalf("Create error = %v, want ErrNotDarwin", err)
	}
}

func TestCreateRequiresOut(t *testing.T) {
	tmp := t.TempDir()
	if _, err := Create(Options{AppDir: fakeBundle(t, tmp), StageDir: filepath.Join(tmp, "s")}); err == nil {
		t.Fatal("Create succeeded with no Out, want an error")
	}
}
