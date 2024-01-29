package main

import (
	"net/url"
	"path"
)

func extractResourceName(urlStr string) (resourceName string, err error) {
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	filename := path.Base(url.Path)
	return filename, nil
}
