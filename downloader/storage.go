package downloader

import (
	"errors"
	"os"
	"path/filepath"
)

func ValidateTargetPath(path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return errors.New("target path does not exits")
	}
	if !fileInfo.IsDir() {
		return errors.New("target path is not a directory")
	}
	return nil
}

// partFilePath is where a download is written while in progress: a hidden
// file next to the output, so the final rename stays on one filesystem.
// The resource name is included so concurrent downloads into the same
// directory don't overwrite each other's part files.
func partFilePath(targetPath, resourceName string) string {
	return filepath.Join(targetPath, "."+resourceName+".part")
}

func outputFilePath(targetPath, resourceName string) string {
	return filepath.Join(targetPath, resourceName)
}

// createPartFile creates (or truncates a stale) part file and extends it to
// size bytes, so every section can write at its own offset.
func createPartFile(path string, size int64) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	if err := file.Truncate(size); err != nil {
		file.Close()
		os.Remove(path)
		return nil, err
	}
	return file, nil
}
