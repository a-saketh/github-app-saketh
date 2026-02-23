package main

// Repository represents a GitHub repository
type Repository struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name string `json:"name"`
}

// PullRequest represents a GitHub pull request
type PullRequest struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
}

// WebhookPayload represents the payload from GitHub webhook
type WebhookPayload struct {
	Action       string      `json:"action"`
	PullRequest  PullRequest `json:"pull_request"`
	Repository   Repository  `json:"repository"`
}

// InstallationToken represents a GitHub App installation token response
type InstallationToken struct {
	Token             string            `json:"token"`
	ExpiresAt         string            `json:"expires_at"`
	Permissions       map[string]string `json:"permissions"`
	RepositorySelection string          `json:"repository_selection"`
}


