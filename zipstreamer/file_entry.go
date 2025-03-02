package zipstreamer

import (
	"errors"
	"net/url"
	"os"
	"path"
	"strings"
)

type FileEntry struct {
	url     *url.URL
	zipPath string
}

const UrlPrefixEnvVar = "ZS_URL_PREFIX"

func NewFileEntry(urlString string, zipPath string) (*FileEntry, error) {
	zipPath = path.Clean(zipPath)
	if path.IsAbs(zipPath) {
		return nil, errors.New("zip path must be relative")
	}

	// ✅ Allow empty folders (directories ending with '/')
	if strings.HasSuffix(zipPath, "/") {
		return &FileEntry{
			url:     nil, // No URL needed for empty directories
			zipPath: zipPath,
		}, nil
	}

	// Validate file entries with URL
	url, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	if url.Scheme != "http" && url.Scheme != "https" {
		return nil, errors.New("url must be a http url")
	}

	urlPrefix := os.Getenv(UrlPrefixEnvVar)
	if !strings.HasPrefix(urlString, urlPrefix) {
		return nil, errors.New("URL not allowed")
	}

	return &FileEntry{url: url, zipPath: zipPath}, nil
}

func (f *FileEntry) Url() *url.URL {
	return f.url
}

func (f *FileEntry) ZipPath() string {
	return f.zipPath
}
