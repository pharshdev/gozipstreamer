package zipstreamer

import (
	"encoding/json"
	"strings"
)

type ZipDescriptor struct {
	suggestedFilenameRaw string
	files                []*FileEntry
}

func NewZipDescriptor() *ZipDescriptor {
	return &ZipDescriptor{
		suggestedFilenameRaw: "",
		files:                make([]*FileEntry, 0),
	}
}

// ✅ Ensures the filename is valid for ZIP download
func (zd ZipDescriptor) EscapedSuggestedFilename() string {
	rawFilename := zd.suggestedFilenameRaw
	escapedFilenameBuilder := make([]rune, 0, len(rawFilename))
	for _, r := range rawFilename {
		if r > 31 && r < 127 && r != '"' {
			escapedFilenameBuilder = append(escapedFilenameBuilder, r)
		}
	}
	escapedFilename := string(escapedFilenameBuilder)

	// Ensure it ends with .zip
	if escapedFilename != "" && escapedFilename != ".zip" {
		if strings.HasSuffix(escapedFilename, ".zip") {
			return escapedFilename
		}
		return escapedFilename + ".zip"
	}
	return "archive.zip"
}

func (zd ZipDescriptor) Files() []*FileEntry {
	return zd.files
}

type jsonZipEntry struct {
	Url     string `json:"url"`
	ZipPath string `json:"zipPath"`
}

type jsonZipPayload struct {
	Files             []jsonZipEntry `json:"files"`
	SuggestedFilename string         `json:"suggestedFilename"`
}

func UnmarshalJsonZipDescriptor(payload []byte) (*ZipDescriptor, error) {
	var parsed jsonZipPayload
	err := json.Unmarshal(payload, &parsed)
	if err != nil {
		return nil, err
	}

	zd := NewZipDescriptor()
	zd.suggestedFilenameRaw = parsed.SuggestedFilename

	for _, jsonZipFileItem := range parsed.Files {
		// ✅ Allow empty folders (without URLs)
		if jsonZipFileItem.Url == "" && !strings.HasSuffix(jsonZipFileItem.ZipPath, "/") {
			continue
		}

		fileEntry, err := NewFileEntry(jsonZipFileItem.Url, jsonZipFileItem.ZipPath)
		if err == nil {
			zd.files = append(zd.files, fileEntry)
		}
	}

	return zd, nil
}
