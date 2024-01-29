package utils

import (
	"net/url"
	"path"
)

func ExtractResourceName(urlStr string) (resourceName string, err error) {
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	filename := path.Base(url.Path)
	return filename, nil
}
