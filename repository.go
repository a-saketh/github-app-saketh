package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
)

// RepositoryContent represents a file or folder in a GitHub repository
type RepositoryContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"` // "file" or "dir"
	Size        int    `json:"size"`
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
}

// FileTreeResult holds the results of the file tree retrieval
type FileTreeResult struct {
	TotalFiles int
	TotalDirs  int
	Files      []string
	Dirs       []string
	AllPaths   []string
}

// getRepositoryFileTree recursively retrieves all files from a GitHub repository
func getRepositoryFileTree(token string, owner string, repo string, path string, result *FileTreeResult) error {
	// GitHub API endpoint for repository contents
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path)

	log.Printf("Fetching from: %s\n", url)

	// Make authenticated request
	body, err := makeAuthenticatedRequest(token, "GET", url, nil)
	if err != nil {
		log.Println("Error: Failed to get repository contents:", err)
		return err
	}

	log.Printf("Response length: %d bytes\n", len(body))
	log.Printf("Response: %s\n", string(body))

	// Parse response
	var contents []RepositoryContent
	if err := json.Unmarshal(body, &contents); err != nil {
		log.Println("Error: Failed to parse contents:", err)
		return err
	}

	log.Printf("Found %d items in %s\n", len(contents), path)

	// Process each item
	for _, item := range contents {
		result.AllPaths = append(result.AllPaths, item.Path)

		if item.Type == "dir" {
			result.TotalDirs++
			result.Dirs = append(result.Dirs, item.Path)
			// Recursively get contents of subdirectory
			if err := getRepositoryFileTree(token, owner, repo, item.Path, result); err != nil {
				log.Printf("Warning: Failed to get contents of %s: %v\n", item.Path, err)
				// Continue with other items
				continue
			}
		} else if item.Type == "file" {
			result.TotalFiles++
			result.Files = append(result.Files, item.Path)
		}
	}

	return nil
}

// GetRepositoryFilesHandler retrieves and lists all files in a GitHub repository
func GetRepositoryFilesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("=== Getting Repository File List ===")

	// Get query parameters
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")

	if owner == "" || repo == "" {
		http.Error(w, "owner and repo parameters are required", http.StatusBadRequest)
		return
	}

	log.Printf("Retrieving files from %s/%s\n", owner, repo)

	// Get GitHub App credentials
	appID := getAppIDFromEnv()
	privateKey := getPrivateKeyFromEnv()

	if appID == "" {
		http.Error(w, "GITHUB_APP_ID not configured", http.StatusInternalServerError)
		return
	}

	if privateKey == "" {
		http.Error(w, "GITHUB_PRIVATE_KEY not configured", http.StatusInternalServerError)
		return
	}

	// Step 1: Generate JWT token
	log.Println("Step 1: Generating JWT token...")
	jwtToken, err := generateJWT(appID, privateKey)
	if err != nil {
		log.Println("Error: Failed to generate JWT:", err)
		http.Error(w, "Failed to generate JWT", http.StatusInternalServerError)
		return
	}
	log.Println("✓ JWT token generated")

	// Step 2: Get installation token
	log.Println("Step 2: Getting installation token...")
	installationToken, err := getInstallationToken(jwtToken, owner, repo)
	if err != nil {
		log.Println("Error: Failed to get installation token:", err)
		http.Error(w, "Failed to get installation token", http.StatusInternalServerError)
		return
	}
	log.Println("✓ Installation token obtained")

	// Step 3: Retrieve file tree
	log.Println("Step 3: Retrieving repository file tree...")
	result := &FileTreeResult{
		Files:    []string{},
		Dirs:     []string{},
		AllPaths: []string{},
	}

	if err := getRepositoryFileTree(installationToken, owner, repo, "", result); err != nil {
		log.Println("Error: Failed to retrieve file tree:", err)
		http.Error(w, "Failed to retrieve file tree", http.StatusInternalServerError)
		return
	}

	// Sort results for consistent output
	sort.Strings(result.Files)
	sort.Strings(result.Dirs)
	sort.Strings(result.AllPaths)

	// Log results
	log.Println("✓ Repository file tree retrieved successfully!")
	log.Printf("Total Files: %d\n", result.TotalFiles)
	log.Printf("Total Directories: %d\n", result.TotalDirs)
	log.Printf("Total Items: %d\n", result.TotalFiles+result.TotalDirs)

	log.Println("\n=== File Paths ===")
	for _, file := range result.Files {
		log.Println("  [FILE]", file)
	}

	log.Println("\n=== Directory Paths ===")
	for _, dir := range result.Dirs {
		log.Println("  [DIR]", dir)
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":              "success",
		"message":             "Repository file tree retrieved successfully",
		"owner":               owner,
		"repo":                repo,
		"total_files":         result.TotalFiles,
		"total_directories":   result.TotalDirs,
		"total_items":         result.TotalFiles + result.TotalDirs,
		"files":               result.Files,
		"directories":         result.Dirs,
	})
}
