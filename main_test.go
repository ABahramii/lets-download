package main

import "testing"

func TestNewDownload(t *testing.T) {
	d, err := newDownload("https://a.com/dir/file.zip", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.URL != "https://a.com/dir/file.zip" || d.TargetPath != "/tmp" || d.ResourceName != "file.zip" || d.TotalSections != 10 {
		t.Fatalf("unexpected download: %+v", d)
	}
}

func TestNewDownload_Invalid(t *testing.T) {
	tests := []struct {
		url     string
		wantErr string
	}{
		{url: "foo bar", wantErr: "URL is invalid"},
		{url: "ftp://a.com/file", wantErr: "URL is invalid"},
		{url: "http:///file", wantErr: "URL is invalid"},
		{url: "http://a.com/%zz", wantErr: "URL is invalid"},
		{url: "http://a.com/", wantErr: "no resource name found in URL"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			_, err := newDownload(tt.url, "/tmp")
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("got %v, want %q", err, tt.wantErr)
			}
		})
	}
}
