package zipstreamer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ✅ Define the ZipStream struct
type ZipStream struct {
	entries           []*FileEntry
	destination       io.Writer
	CompressionMethod uint16
}

// ✅ Constructor function to create a new ZipStream
func NewZipStream(entries []*FileEntry, w io.Writer) (*ZipStream, error) {
	if len(entries) == 0 {
		return nil, errors.New("must have at least 1 entry")
	}

	return &ZipStream{
		entries:           entries,
		destination:       w,
		CompressionMethod: zip.Store, // Default to no compression
	}, nil
}

func (z *ZipStream) StreamAllFiles() error {
	zipWriter := zip.NewWriter(z.destination)
	success := 0

	for _, entry := range z.entries {
		// ✅ Explicitly add empty folders to the ZIP
		if entry.Url() == nil {
			folderPath := entry.ZipPath()
			if !strings.HasSuffix(folderPath, "/") {
				folderPath += "/"
			}

			fmt.Printf("Adding empty folder to ZIP: %s\n", folderPath) // Debugging log

			header := &zip.FileHeader{
				Name:     folderPath,
				Method:   zip.Store, // No compression for folders
				Modified: time.Now(),
			}
			header.SetMode(os.ModeDir | 0755) // ✅ Ensure it's treated as a directory

			_, err := zipWriter.CreateHeader(header)
			if err != nil {
				return fmt.Errorf("failed to create directory entry %s: %v", folderPath, err)
			}

			success++
			continue
		}

		// ✅ Handle files as usual
		resp, err := http.Get(entry.Url().String())
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}

		header := &zip.FileHeader{
			Name:     entry.ZipPath(),
			Method:   z.CompressionMethod,
			Modified: time.Now(),
		}
		entryWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		_, err = io.Copy(entryWriter, resp.Body)
		if err != nil {
			return err
		}

		zipWriter.Flush()
		flushingWriter, ok := z.destination.(http.Flusher)
		if ok {
			flushingWriter.Flush()
		}

		success++
	}

	// ✅ Ensure at least one entry (file or folder) is added, otherwise return an error
	if err := zipWriter.Close(); err != nil {
		return err
	}

	if success == 0 {
		return errors.New("empty file - all files and folders failed")
	}

	return nil
}
