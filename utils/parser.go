package utils

import (
	"errors"
	"net/url"
	"os"
	"path"
)

func ExtractResourceName(urlStr string) (resourceName string, err error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", errors.New("URL is invalid")
	}

	// prefer `filename` query parameter if provided
	if q := u.Query().Get("filename"); q != "" {
		return path.Base(q), nil
	}

	// fallback to the path's base name
	filename := path.Base(u.Path)
	if filename == "" || filename == "." || filename == "/" {
		return "", errors.New("no resource name found in URL")
	}
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
