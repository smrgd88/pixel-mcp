//go:build linux || darwin

package aseprite

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func tryFileLock(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}
func replaceFile(src, dst string) error { return os.Rename(src, dst) }
func checkSingleLink(path string, info os.FileInfo) error {
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return fmt.Errorf("hard-linked file cannot be atomically replaced: %s", path)
	}
	return nil
}
