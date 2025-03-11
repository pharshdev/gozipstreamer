package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"gozipstreamer/zipstreamer"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
)

// File size mapping (ZipPath -> Size)
var fileSizeMap map[string]int64

// APIResponse represents the structure of the API response from Premiumize.me
type APIResponse struct {
	Status  string `json:"status"`
	Content []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
		DirectLink string `json:"directlink,omitempty"`
		Size       int64  `json:"size"`
	} `json:"content"`
	Name     string `json:"name"`
	FolderID string `json:"folder_id"`
	ParentID string `json:"parent_id"`
}

// fetchFolderContents retrieves the contents of a folder from Premiumize.me API
func fetchFolderContents(apiKey, path string) (*APIResponse, error) {
	encodedPath := strings.ReplaceAll(path, " ", "%20") // Encode spaces
	apiURL := fmt.Sprintf("https://www.premiumize.me/api/folder/list?apikey=%s&path=%s", apiKey, encodedPath)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch folder contents: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	var apiResponse APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON response: %v", err)
	}

	if apiResponse.Status != "success" {
		return nil, fmt.Errorf("API response status: %s", apiResponse.Status)
	}

	return &apiResponse, nil
}

// traverseFolder recursively builds the file list & tracks sizes
func traverseFolder(apiKey, path, parentZipPath string, files *[]*zipstreamer.FileEntry, rootPath string) error {
	apiResponse, err := fetchFolderContents(apiKey, path)
	if err != nil {
		return err
	}

	var relativeZipPath string
	if path == rootPath {
		relativeZipPath = filepath.Base(rootPath)
	} else {
		relativeZipPath = filepath.Join(parentZipPath, apiResponse.Name)
	}

	for _, item := range apiResponse.Content {
		currentZipPath := filepath.Join(relativeZipPath, item.Name)

		if item.Type == "file" {
			entry, err := zipstreamer.NewFileEntry(item.DirectLink, currentZipPath)
			if err == nil {
				*files = append(*files, entry)
				fileSizeMap[currentZipPath] = item.Size // Store file size in map
			}
		} else if item.Type == "folder" {
			err := traverseFolder(apiKey, filepath.Join(path, item.Name), relativeZipPath, files, rootPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// calculateZipSize computes the estimated ZIP file size
func calculateZipSize(files []*zipstreamer.FileEntry) (int64, int64, int64, int64) {
	const localHeaderSize = 30
	const centralDirSize = 46
	const eocdSize = 22

	var totalLocalHeaders int64
	var totalFileData int64
	var totalCentralDir int64

	for _, file := range files {
		var zipPath string
		if zipPathMethod, ok := interface{}(file).(interface{ ZipPath() string }); ok {
			zipPath = zipPathMethod.ZipPath()
		} else {
			zipPath = file.ZipPath()
		}

		filenameLen := int64(len(zipPath))
		fileSize := fileSizeMap[zipPath]

		totalLocalHeaders += localHeaderSize + filenameLen
		totalFileData += fileSize
		totalCentralDir += centralDirSize + filenameLen
	}

	totalZipSize := totalLocalHeaders + totalFileData + totalCentralDir + eocdSize

	// Log the size breakdown
	fmt.Printf("ZIP Size Breakdown:\n")
	fmt.Printf("  - Local Headers: %d bytes\n", totalLocalHeaders)
	fmt.Printf("  - File Data: %d bytes\n", totalFileData)
	fmt.Printf("  - Central Directory: %d bytes\n", totalCentralDir)
	fmt.Printf("  - End of Central Directory: %d bytes\n", eocdSize)
	fmt.Printf("  - Total ZIP Size: %d bytes\n", totalZipSize)

	return totalZipSize, totalLocalHeaders, totalFileData, totalCentralDir
}

// zipHandler handles API requests to generate ZIP
func zipHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		apiKey := r.URL.Query().Get("apikey")
		pathsParam := r.URL.Query().Get("paths")

		if apiKey == "" || pathsParam == "" {
			http.Error(w, "Missing API key or paths", http.StatusBadRequest)
			return
		}

		var paths []string
		err := json.Unmarshal([]byte(pathsParam), &paths)
		if err != nil {
			http.Error(w, "Invalid paths parameter", http.StatusBadRequest)
			return
		}

		processZipRequest(w, apiKey, paths)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// Function to handle ZIP processing
func processZipRequest(w http.ResponseWriter, apiKey string, paths []string) {
	fileSizeMap = make(map[string]int64) // Initialize file size map
	var fileEntries []*zipstreamer.FileEntry

	// Recursively fetch all files and subfolders
	for _, rootPath := range paths {
		fmt.Printf("Processing folder: %s\n", rootPath)
		err := traverseFolder(apiKey, rootPath, "", &fileEntries, rootPath)
		if err != nil {
			fmt.Printf("Error processing %s: %v\n", rootPath, err)
		}
	}

	// Handle empty folder case
	if len(fileEntries) == 0 {
		fmt.Println("Empty folder detected. Returning an empty ZIP.")
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=empty.zip")

		zipWriter := zip.NewWriter(w)
		zipWriter.Close()
		return
	}

	// Compute ZIP size breakdown
	zipSize, totalLocalHeaders, totalFileData, totalCentralDir := calculateZipSize(fileEntries)

	// Log the computed ZIP size details
	fmt.Printf("\nFinal ZIP Size: %d bytes\n", zipSize)
	fmt.Printf("  - Headers: %d bytes\n", totalLocalHeaders)
	fmt.Printf("  - Actual File Data: %d bytes\n", totalFileData)
	fmt.Printf("  - Central Directory: %d bytes\n", totalCentralDir)

	// Set headers for ZIP download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=archive.zip")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", zipSize))
	w.Header().Set("Accept-Ranges", "bytes") // Enables Range Requests

	// Create ZIP stream
	zipStream, err := zipstreamer.NewZipStream(fileEntries, w)
	if err != nil {
		http.Error(w, "Failed to create ZIP stream", http.StatusInternalServerError)
		return
	}

	if err := zipStream.StreamAllFiles(); err != nil {
		http.Error(w, "Failed to stream ZIP", http.StatusInternalServerError)
	}
}

func main() {
	r := mux.NewRouter()

	// If serving an HTML page, re-add this:
	r.HandleFunc("/", serveHTML).Methods("GET")

	// Handle ZIP streaming requests
	r.HandleFunc("/create-zip", zipHandler).Methods("GET", "POST")

	fmt.Println("Server started on :80")
	if err := http.ListenAndServe(":80", r); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}
