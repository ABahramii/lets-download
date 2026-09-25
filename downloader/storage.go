package downloader

import (
	"errors"
	"fmt"
	"io"
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

// sectionFilePath includes the resource name so concurrent downloads
// into the same directory don't overwrite each other's temp files.
func sectionFilePath(targetPath, resourceName string, i int) string {
	return filepath.Join(targetPath, fmt.Sprintf("%s.section-%d.tmp", resourceName, i))
}

func outputFilePath(targetPath, resourceName string) string {
	return filepath.Join(targetPath, resourceName+".mp4")
}

func mergeFiles(targetPath, resourceName string, sections []byteRange) error {
	filePath := outputFilePath(targetPath, resourceName)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()
	for i := range sections {
		if err := appendFile(file, sectionFilePath(targetPath, resourceName, i)); err != nil {
			return err
		}
	}
	return nil
}

// appendFile streams the file at path into dst without loading it into memory.
func appendFile(dst io.Writer, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (download *Download) removeTempFiles(sections []byteRange) error {
	for i := range sections {
		err := os.Remove(sectionFilePath(download.TargetPath, download.ResourceName, i))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
