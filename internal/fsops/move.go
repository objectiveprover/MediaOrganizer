// Package fsops moves files safely: never overwrites, and falls back to
// copy+verify+delete across filesystems.
package fsops

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// ErrExists is returned when the destination already exists.
var ErrExists = fs.ErrExist

// renameNoReplace is set per-OS (renameat2 RENAME_NOREPLACE on Linux).
var renameNoReplace = func(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return ErrExists
	}
	return os.Rename(src, dst)
}

// Move moves src to dst, creating parent directories. progress (optional)
// receives bytes copied when a cross-filesystem copy is needed.
func Move(src, dst string, progress func(int64)) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	err := renameNoReplace(src, dst)
	if err == nil {
		return nil
	}
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("destination exists: %s", dst)
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	return copyAcross(src, dst, progress)
}

type progressWriter struct {
	w  io.Writer
	fn func(int64)
}

func (p progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if p.fn != nil && n > 0 {
		p.fn(int64(n))
	}
	return n, err
}

func copyAcross(src, dst string, progress func(int64)) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".renamer-part")
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, st.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			out.Close()
			os.Remove(tmp)
		}
	}()
	buf := make([]byte, 4<<20)
	n, err := io.CopyBuffer(progressWriter{out, progress}, in, buf)
	if err != nil {
		return err
	}
	if n != st.Size() {
		return fmt.Errorf("short copy: %d of %d bytes", n, st.Size())
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if ost, e := os.Stat(tmp); e != nil || ost.Size() != st.Size() {
		return fmt.Errorf("verify failed for %s", tmp)
	}
	_ = os.Chtimes(tmp, st.ModTime(), st.ModTime())
	if err = renameNoReplace(tmp, dst); err != nil {
		if errors.Is(err, fs.ErrExist) {
			err = fmt.Errorf("destination exists: %s", dst)
		}
		return err
	}
	if e := os.Remove(src); e != nil {
		return fmt.Errorf("copied to %s but could not remove source: %w", dst, e)
	}
	return nil
}

// RemoveEmptyDirs removes dir and its parents while they are empty,
// never removing stop itself or anything outside it.
func RemoveEmptyDirs(dir, stop string) {
	stop = filepath.Clean(stop)
	for {
		dir = filepath.Clean(dir)
		if dir == stop || len(dir) <= len(stop) {
			return
		}
		rel, err := filepath.Rel(stop, dir)
		if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || (len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)) {
			return
		}
		if os.Remove(dir) != nil { // fails if not empty
			return
		}
		dir = filepath.Dir(dir)
	}
}
