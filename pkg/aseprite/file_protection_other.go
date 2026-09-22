//go:build !linux && !darwin && !windows

package aseprite

import (
	"fmt"
	"os"
)

func tryFileLock(*os.File) (bool, error) {
	return false, fmt.Errorf("file protection unsupported on this platform")
}
func replaceFile(string, string) error {
	return fmt.Errorf("atomic replace unsupported on this platform")
}
func checkSingleLink(string, os.FileInfo) error {
	return fmt.Errorf("file protection unsupported on this platform")
}

func openReadFile(path string) (*os.File, error) {
	return nil, fmt.Errorf("file protection unsupported on this platform")
}
