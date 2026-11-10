package fsops

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMoveSameFS(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "a", "x.mkv")
	dst := filepath.Join(d, "out", "Movies", "X (2000)", "X (2000).mkv")
	write(t, src, "hello")
	if err := Move(src, dst, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "hello" {
		t.Error("content mismatch")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists")
	}
}

func TestMoveNeverOverwrites(t *testing.T) {
	d := t.TempDir()
	src, dst := filepath.Join(d, "a.mkv"), filepath.Join(d, "b.mkv")
	write(t, src, "new")
	write(t, dst, "old")
	err := Move(src, dst, nil)
	if err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("want exists error, got %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "old" {
		t.Error("destination was overwritten")
	}
}

func TestMoveCrossDevice(t *testing.T) {
	orig := renameNoReplace
	defer func() { renameNoReplace = orig }()
	calls := 0
	renameNoReplace = func(src, dst string) error {
		calls++
		if calls == 1 {
			return &os.LinkError{Op: "rename", Old: src, New: dst, Err: syscall.EXDEV}
		}
		return orig(src, dst)
	}
	d := t.TempDir()
	src := filepath.Join(d, "src", "x.mkv")
	dst := filepath.Join(d, "dst", "y.mkv")
	write(t, src, strings.Repeat("z", 10000))
	var copied int64
	if err := Move(src, dst, func(n int64) { copied += n }); err != nil {
		t.Fatal(err)
	}
	if copied != 10000 {
		t.Errorf("progress reported %d bytes", copied)
	}
	if st, err := os.Stat(dst); err != nil || st.Size() != 10000 {
		t.Errorf("dst: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source not removed")
	}
	if m, _ := filepath.Glob(filepath.Join(d, "dst", ".*renamer-part")); len(m) != 0 {
		t.Error("temp file left behind")
	}
}

func TestRemoveEmptyDirs(t *testing.T) {
	d := t.TempDir()
	deep := filepath.Join(d, "a", "b", "c")
	os.MkdirAll(deep, 0o755)
	write(t, filepath.Join(d, "a", "keep.txt"), "x")
	RemoveEmptyDirs(deep, d)
	if _, err := os.Stat(filepath.Join(d, "a", "b")); !os.IsNotExist(err) {
		t.Error("empty dirs not removed")
	}
	if _, err := os.Stat(filepath.Join(d, "a")); err != nil {
		t.Error("non-empty dir removed")
	}
	RemoveEmptyDirs(d, d)
	if _, err := os.Stat(d); err != nil {
		t.Error("root removed")
	}
}
