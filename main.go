package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gozipstreamer/zipstreamer"

	"github.com/gorilla/mux"
)

// File represents a file entry in the JSON descriptor
type File struct {
	URL     string `json:"url,omitempty"`
	ZipPath string `json:"zipPath"`
}

// APIResponse represents the structure of the API response from Premiumize.me
type APIResponse struct {
	Status  string `json:"status"`
	Content []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
		DirectLink string `json:"directlink,omitempty"`
	} `json:"content"`
	Name     string `json:"name"`
	FolderID string `json:"folder_id"`
	ParentID string `json:"parent_id"`
}

// Serve the HTML page
func serveHTML(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

// fetchFolderContents retrieves the contents of a folder from the Premiumize.me API
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

// traverseFolder recursively traverses the folder and its subfolders to build the file list
func traverseFolder(apiKey, path, parentZipPath string, files *[]*zipstreamer.FileEntry, rootPath string) error {
	apiResponse, err := fetchFolderContents(apiKey, path)
	if err != nil {
		return err
	}

	// Compute correct relative path including the root folder name
	var relativeZipPath string
	if path == rootPath {
		relativeZipPath = filepath.Base(rootPath) // Use the root folder name
	} else {
		relativeZipPath = filepath.Join(parentZipPath, apiResponse.Name)
	}

	// Traverse contents of the folder
	for _, item := range apiResponse.Content {
		currentZipPath := filepath.Join(relativeZipPath, item.Name) // Keeps root folder name

		if item.Type == "file" {
			entry, err := zipstreamer.NewFileEntry(item.DirectLink, currentZipPath)
			if err == nil {
				*files = append(*files, entry) // Append zipstreamer.FileEntry instead of File
			}
		} else if item.Type == "folder" {
			// Pass `relativeZipPath` correctly to maintain folder hierarchy
			err := traverseFolder(apiKey, filepath.Join(path, item.Name), relativeZipPath, files, rootPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// zipHandler handles API requests to generate ZIP
func zipHandler(w http.ResponseWriter, r *http.Request) {
	// Support GET requests
	if r.Method == "GET" {
		apiKey := r.URL.Query().Get("apikey")
		pathsParam := r.URL.Query().Get("paths")

		if apiKey == "" || pathsParam == "" {
			http.Error(w, "Missing API key or paths", http.StatusBadRequest)
			return
		}

		// Decode paths JSON from query string
		var paths []string
		err := json.Unmarshal([]byte(pathsParam), &paths)
		if err != nil {
			http.Error(w, "Invalid paths parameter", http.StatusBadRequest)
			return
		}

		// Call the function to process paths
		processZipRequest(w, apiKey, paths)
		return
	}

	// Support POST requests (for API calls)
	if r.Method == "POST" {
		var requestData struct {
			ApiKey string   `json:"apikey"`
			Paths  []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
			http.Error(w, "Invalid JSON request", http.StatusBadRequest)
			return
		}

		if requestData.ApiKey == "" || len(requestData.Paths) == 0 {
			http.Error(w, "Missing API key or paths", http.StatusBadRequest)
			return
		}

		// Call the function to process paths
		processZipRequest(w, requestData.ApiKey, requestData.Paths)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// Function to handle ZIP processing (directly using `zipstreamer`)
func processZipRequest(w http.ResponseWriter, apiKey string, paths []string) {
	var fileEntries []*zipstreamer.FileEntry

	for _, rootPath := range paths {
		fmt.Printf("Processing folder: %s\n", rootPath)
		err := traverseFolder(apiKey, rootPath, "", &fileEntries, rootPath) // Pass correct type
		if err != nil {
			fmt.Printf("Error processing %s: %v\n", rootPath, err)
		}
	}

	// Create ZIP stream
	zipStream, err := zipstreamer.NewZipStream(fileEntries, w)
	if err != nil {
		http.Error(w, "Failed to create ZIP stream", http.StatusInternalServerError)
		return
	}

	// Set headers for ZIP download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=archive.zip")

	// Stream ZIP file to client
	if err := zipStream.StreamAllFiles(); err != nil {
		http.Error(w, "Failed to stream ZIP", http.StatusInternalServerError)
	}
}

func main() {
	// Set up the API server
	r := mux.NewRouter()

	// Serve the web page
	r.HandleFunc("/", serveHTML).Methods("GET")

	// Serve static files (for CSS, JS, images if needed)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Handle ZIP creation
	r.HandleFunc("/create-zip", zipHandler).Methods("GET", "POST")

	fmt.Println("Server started on :80")
	if err := http.ListenAndServe(":80", r); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
}
