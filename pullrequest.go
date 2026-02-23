package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

// PRFile represents a file changed in a pull request
type PRFile struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"` // "added", "removed", "modified", "renamed"
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Changes     int    `json:"changes"`
	PreviousFilename string `json:"previous_filename"` // only set when status = "renamed"
}

// getPRChangedFiles fetches the list of files changed in a pull request
func getPRChangedFiles(token string, owner string, repo string, prNumber int) ([]PRFile, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/files", owner, repo, prNumber)
	log.Printf("Fetching PR files from: %s\n", url)

	body, err := makeAuthenticatedRequest(token, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR files: %w", err)
	}

	// Check for API error response
	var apiErr map[string]interface{}
	if err := json.Unmarshal(body, &apiErr); err == nil {
		if msg, ok := apiErr["message"]; ok {
			return nil, fmt.Errorf("GitHub API error: %v", msg)
		}
	}

	var files []PRFile
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("failed to parse PR files: %w", err)
	}

	return files, nil
}

// logPRChangedFiles logs the changed files in a structured way
func logPRChangedFiles(files []PRFile) {
	log.Printf("=== PR Changed Files (%d total) ===\n", len(files))
	for _, f := range files {
		if f.Status == "renamed" {
			log.Printf("  [%s] %s -> %s (+%d -%d)\n", f.Status, f.PreviousFilename, f.Filename, f.Additions, f.Deletions)
		} else {
			log.Printf("  [%s] %s (+%d -%d)\n", f.Status, f.Filename, f.Additions, f.Deletions)
		}
	}
}

// GetPRFilesHandler is an HTTP endpoint to retrieve changed files in a PR
func GetPRFilesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("=== Getting PR Changed Files ===")

	// Get query parameters
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")
	prNumberStr := r.URL.Query().Get("pr")

	if owner == "" || repo == "" || prNumberStr == "" {
		http.Error(w, "owner, repo and pr parameters are required", http.StatusBadRequest)
		return
	}

	prNumber, err := strconv.Atoi(prNumberStr)
	if err != nil {
		http.Error(w, "pr must be a valid number", http.StatusBadRequest)
		return
	}

	log.Printf("Retrieving changed files for PR #%d in %s/%s\n", prNumber, owner, repo)

	// Authenticate with GitHub
	appID := getAppIDFromEnv()
	privateKey := getPrivateKeyFromEnv()

	if appID == "" || privateKey == "" {
		http.Error(w, "GitHub App credentials not configured", http.StatusInternalServerError)
		return
	}

	// Step 1: Generate JWT
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

	// Step 3: Fetch changed files
	log.Println("Step 3: Fetching changed files in PR...")
	files, err := getPRChangedFiles(installationToken, owner, repo, prNumber)
	if err != nil {
		log.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log results
	logPRChangedFiles(files)

	// Build summary
	filenames := make([]string, len(files))
	for i, f := range files {
		filenames[i] = f.Filename
	}

	// Calculate total changes
	totalAdditions, totalDeletions, totalChanges := 0, 0, 0
	for _, f := range files {
		totalAdditions += f.Additions
		totalDeletions += f.Deletions
		totalChanges += f.Changes
	}

	log.Printf("✓ Total: %d files changed, +%d -%d lines\n", len(files), totalAdditions, totalDeletions)

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "success",
		"owner":           owner,
		"repo":            repo,
		"pr_number":       prNumber,
		"total_files":     len(files),
		"total_additions": totalAdditions,
		"total_deletions": totalDeletions,
		"total_changes":   totalChanges,
		"filenames":       filenames,
		"files":           files,
	})
}
