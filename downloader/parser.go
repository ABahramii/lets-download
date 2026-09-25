package downloader

import (
	"errors"
	"net/url"
	"path"
)

var errInvalidURL = errors.New("URL is invalid")

func ExtractResourceName(urlStr string) (resourceName string, err error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", errInvalidURL
	}
	return resourceNameFromURL(u)
}

func resourceNameFromURL(u *url.URL) (string, error) {
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
