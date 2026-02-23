package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// handler is the basic HTTP handler for the root endpoint
func handler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		w.Write([]byte("Hello from POST"))
	} else if req.Method == "GET" {
		w.Write([]byte("Hello from GET"))
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// AuthTestHandler demonstrates the full GitHub App authentication flow
func AuthTestHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("=== Testing GitHub App Authentication ===")

	// Get configuration from environment
	appID := os.Getenv("GITHUB_APP_ID")
	privateKey := os.Getenv("GITHUB_PRIVATE_KEY")

	if appID == "" {
		log.Println("Error: GITHUB_APP_ID not set")
		http.Error(w, "GITHUB_APP_ID not configured", http.StatusInternalServerError)
		return
	}

	if privateKey == "" {
		log.Println("Error: GITHUB_PRIVATE_KEY not set")
		http.Error(w, "GITHUB_PRIVATE_KEY not configured", http.StatusInternalServerError)
		return
	}

	// Generate JWT token
	log.Println("Step 1: Generating JWT token...")
	jwtToken, err := generateJWT(appID, privateKey)
	if err != nil {
		log.Println("Error: Failed to generate JWT:", err)
		http.Error(w, "Failed to generate JWT", http.StatusInternalServerError)
		return
	}
	log.Println("✓ JWT token generated successfully")
	log.Println("JWT Token (first 50 chars):", jwtToken[:50]+"...")

	// Get installation token
	log.Println("Step 2: Getting installation token...")
	installationToken, err := getInstallationToken(jwtToken, "", "")
	if err != nil {
		log.Println("Error: Failed to get installation token:", err)
		http.Error(w, "Failed to get installation token", http.StatusInternalServerError)
		return
	}
	log.Println("✓ Installation token obtained successfully")
	log.Println("Installation Token (first 50 chars):", installationToken[:50]+"...")

	// Test an authenticated API request (get authenticated user)
	log.Println("Step 3: Making authenticated API request...")
	responseBody, err := makeAuthenticatedRequest(installationToken, "GET", "https://api.github.com/user", nil)
	if err != nil {
		log.Println("Error: Failed to make authenticated request:", err)
		http.Error(w, "Failed to make authenticated request", http.StatusInternalServerError)
		return
	}

	var userInfo map[string]interface{}
	if err := json.Unmarshal(responseBody, &userInfo); err != nil {
		log.Println("Error: Failed to parse user info:", err)
		http.Error(w, "Failed to parse response", http.StatusInternalServerError)
		return
	}

	log.Println("✓ Authenticated API request successful!")
	log.Println("User Info:", userInfo)

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"message":      "GitHub App authentication successful",
		"app_id":       appID,
		"jwt_preview":  jwtToken[:50] + "...",
	})
}
