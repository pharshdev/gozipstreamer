package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

const (
	apiKey     = "2x5dfzb2cm69f5c6" // Replace with your actual Premiumize.me API key
	apiBaseURL = "https://www.premiumize.me/api/folder/list"
)

// File represents a file entry in the JSON descriptor
type File struct {
	URL     string `json:"url"`
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
		DirectLink string `json:"directlink"`
	} `json:"content"`
}

// fetchFolderContents retrieves the contents of a folder from the Premiumize.me API
func fetchFolderContents(path string) (*APIResponse, error) {
	encodedPath := url.QueryEscape(path)
	apiURL := fmt.Sprintf("%s?apikey=%s&path=%s", apiBaseURL, apiKey, encodedPath)

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
func traverseFolder(path, parentZipPath string, files *[]File) error {
	apiResponse, err := fetchFolderContents(path)
	if err != nil {
		return err
	}

	for _, item := range apiResponse.Content {
		currentZipPath := filepath.Join(parentZipPath, item.Name)
		if item.Type == "file" {
			*files = append(*files, File{
				URL:     item.DirectLink,
				ZipPath: currentZipPath,
			})
		} else if item.Type == "folder" {
			if err := traverseFolder(filepath.Join(path, item.Name), currentZipPath, files); err != nil {
				return err
			}
		}
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <path>")
		return
	}

	rootPath := os.Args[1]
	var files []File

	if err := traverseFolder(rootPath, "", &files); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	descriptor := Descriptor{
		SuggestedFilename: "archive.zip",
		Files:             files,
	}

	jsonData, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		fmt.Printf("Failed to marshal JSON: %v\n", err)
		return
	}

	if err := ioutil.WriteFile("descriptor.json", jsonData, 0644); err != nil {
		fmt.Printf("Failed to write JSON to file: %v\n", err)
		return
	}

	fmt.Println("JSON descriptor created successfully: descriptor.json")
}
