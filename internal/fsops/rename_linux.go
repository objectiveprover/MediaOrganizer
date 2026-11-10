//go:build linux

package fsops

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func init() {
	fallback := renameNoReplace
	renameNoReplace = func(src, dst string) error {
		err := unix.Renameat2(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.RENAME_NOREPLACE)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, unix.EEXIST):
			return os.ErrExist
		case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EOPNOTSUPP):
			// Filesystem doesn't support RENAME_NOREPLACE (some network FS).
			return fallback(src, dst)
		}
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
}
