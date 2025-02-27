package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
)

const (
	zipStreamerURL = "http://localhost:8080/download" // ZipStreamer API URL
)

// File represents a file entry in the JSON descriptor
type File struct {
	URL     string `json:"url,omitempty"`
	ZipPath string `json:"zipPath"`
}

// Descriptor represents the JSON structure required by ZipStreamer
type Descriptor struct {
	SuggestedFilename string `json:"suggestedFilename"`
	Files             []File `json:"files"`
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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var apiResponse APIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON response: %v", err)
	}

	if apiResponse.Status != "success" {
		return nil, fmt.Errorf("API response status: %s", apiResponse.Status)
	}

	return &apiResponse, nil
}

// traverseFolder recursively traverses the folder and its subfolders to build the file list
func traverseFolder(apiKey, path, parentZipPath string, files *[]File, rootPath string) error {
	apiResponse, err := fetchFolderContents(apiKey, path)
	if err != nil {
		return err
	}

	// ✅ Compute correct relative path including the root folder name
	var relativeZipPath string
	if path == rootPath {
		relativeZipPath = filepath.Base(rootPath) // ✅ Use the root folder name
	} else {
		relativeZipPath = filepath.Join(parentZipPath, apiResponse.Name)
	}

	// ✅ Ignore empty folders
	if len(apiResponse.Content) == 0 {
		return nil
	}

	// ✅ Traverse contents of the folder
	for _, item := range apiResponse.Content {
		currentZipPath := filepath.Join(relativeZipPath, item.Name) // ✅ Keeps root folder name

		if item.Type == "file" {
			*files = append(*files, File{
				URL:     item.DirectLink,
				ZipPath: currentZipPath,
			})
		} else if item.Type == "folder" {
			// ✅ Pass `relativeZipPath` correctly to maintain folder hierarchy
			err := traverseFolder(apiKey, filepath.Join(path, item.Name), relativeZipPath, files, rootPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// createDescriptor generates descriptor.json from multiple paths
func createDescriptor(apiKey string, paths []string) (*Descriptor, error) {
	var files []File

	// ✅ Process multiple paths
	for _, rootPath := range paths {
		fmt.Printf("Processing folder: %s\n", rootPath)
		err := traverseFolder(apiKey, rootPath, "", &files, rootPath)
		if err != nil {
			fmt.Printf("Error processing %s: %v\n", rootPath, err)
		}
	}

	descriptor := &Descriptor{
		SuggestedFilename: "archive.zip",
		Files:             files,
	}

	return descriptor, nil
}

// zipHandler handles API requests to generate ZIP
func zipHandler(w http.ResponseWriter, r *http.Request) {
	// ✅ Parse request body
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

	// ✅ Generate descriptor.json dynamically
	descriptor, err := createDescriptor(requestData.ApiKey, requestData.Paths)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate descriptor: %v", err), http.StatusInternalServerError)
		return
	}

	// ✅ Convert descriptor to JSON
	descriptorJSON, err := json.Marshal(descriptor)
	if err != nil {
		http.Error(w, "Failed to marshal descriptor JSON", http.StatusInternalServerError)
		return
	}

	// ✅ Send descriptor.json to ZipStreamer
	fmt.Println("Sending descriptor to ZipStreamer...")
	resp, err := http.Post(zipStreamerURL, "application/json", bytes.NewBuffer(descriptorJSON))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to ZipStreamer: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// ✅ Debug response from ZipStreamer
	fmt.Printf("ZipStreamer Response Status: %s\n", resp.Status)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Printf("ZipStreamer Response Body: %s\n", string(body))

	// ✅ If ZipStreamer returns an error, send it to the client
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("ZipStreamer Error: %s", string(body)), http.StatusBadGateway)
		return
	}

	// ✅ Stream ZIP response back to client
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=archive.zip")
	io.Copy(w, bytes.NewReader(body)) // Ensure proper streaming
}

func main() {
	// ✅ Set up the API server
	r := mux.NewRouter()
	r.HandleFunc("/create-zip", zipHandler).Methods("POST")

	fmt.Println("Server started on :8000")

	// ✅ Keep the server running and handle errors
	if err := http.ListenAndServe(":8000", r); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1) // ✅ Exit with error if server fails
	}
}
