package utils

import (
	"errors"
	"net/url"
	"os"
	"path"
)

func ExtractResourceName(urlStr string) (resourceName string, err error) {
	parse, err := url.Parse(urlStr)
	if err != nil {
		return "", errors.New("URL is invalid")
	}
	filename := path.Base(parse.Path)
	return filename, nil
}

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
