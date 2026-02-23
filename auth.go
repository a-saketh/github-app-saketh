package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// getAppIDFromEnv retrieves the GitHub App ID from environment
func getAppIDFromEnv() string {
	return os.Getenv("GITHUB_APP_ID")
}

// getPrivateKeyFromEnv retrieves the GitHub App private key from environment
func getPrivateKeyFromEnv() string {
	return os.Getenv("GITHUB_PRIVATE_KEY")
}

// generateJWT creates a JWT token for GitHub App authentication
func generateJWT(appID string, privateKeyPEM string) (string, error) {
	// Parse private key
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		log.Println("Error: Failed to parse private key PEM")
		return "", fmt.Errorf("failed to parse private key PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		log.Println("Error: Failed to parse private key:", err)
		return "", err
	}

	// Parse app ID from string
	var appIDInt int64
	if _, err := fmt.Sscanf(appID, "%d", &appIDInt); err != nil {
		log.Println("Error: Invalid App ID format:", err)
		return "", err
	}

	// Use MapClaims to have full control over the JWT fields
	// GitHub requires: iss = app ID (int), iat = now, exp = now + max 10 min
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iss": appIDInt,
		"iat": now,
		"exp": now + 540, // 9 minutes — safely under GitHub's 10-minute max
	}

	// Create and sign token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		log.Println("Error: Failed to sign JWT:", err)
		return "", err
	}

	return tokenString, nil
}

// getInstallationToken exchanges JWT for an installation token
func getInstallationToken(jwtToken string, owner string, repo string) (string, error) {
	// Get the app ID from environment
	appID := os.Getenv("GITHUB_APP_ID")

	// List installations endpoint
	url := "https://api.github.com/app/installations"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-App-"+appID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error: Failed to get installations:", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Println("Error: GitHub API returned", resp.StatusCode, ":", string(body))
		return "", err
	}

	// Parse installations
	var installations []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&installations); err != nil {
		log.Println("Error: Failed to parse installations:", err)
		return "", err
	}

	if len(installations) == 0 {
		log.Println("Error: No installations found")
		return "", nil
	}

	// Get the first installation's token endpoint
	installationID := int(installations[0]["id"].(float64))
	tokenURL := "https://api.github.com/app/installations/" + fmt.Sprintf("%d", installationID) + "/access_tokens"

	req, err = http.NewRequest("POST", tokenURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-App-"+appID)

	resp, err = client.Do(req)
	if err != nil {
		log.Println("Error: Failed to get installation token:", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Println("Error: GitHub API returned", resp.StatusCode, ":", string(body))
		return "", err
	}

	// Parse token response
	var tokenResp InstallationToken
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		log.Println("Error: Failed to parse token response:", err)
		return "", err
	}

	return tokenResp.Token, nil
}

// makeAuthenticatedRequest makes an authenticated API request to GitHub
func makeAuthenticatedRequest(token string, method string, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		bodyBytes, _ := json.Marshal(body)
		reqBody = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-App")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
