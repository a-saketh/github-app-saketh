package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// GitHubAdapter implements SCMAdapter for GitHub.
// It reuses the existing JWT / installation-token auth layer in auth.go and
// the PR file fetching logic in pullrequest.go.
type GitHubAdapter struct {
	appID      string
	privateKey string
}

// NewGitHubAdapter creates a GitHubAdapter from environment credentials.
// Required env vars: GITHUB_APP_ID, GITHUB_PRIVATE_KEY.
func NewGitHubAdapter() (*GitHubAdapter, error) {
	appID := getAppIDFromEnv()
	privateKey := getPrivateKeyFromEnv()
	if appID == "" || privateKey == "" {
		return nil, fmt.Errorf("GitHub adapter: GITHUB_APP_ID and GITHUB_PRIVATE_KEY must be set")
	}
	return &GitHubAdapter{appID: appID, privateKey: privateKey}, nil
}

func (g *GitHubAdapter) Platform() SCMPlatform {
	return PlatformGitHub
}

// token generates a short-lived installation access token for the given repo.
func (g *GitHubAdapter) token(owner, repo string) (string, error) {
	jwtToken, err := generateJWT(g.appID, g.privateKey)
	if err != nil {
		return "", fmt.Errorf("GitHub adapter: failed to generate JWT: %w", err)
	}
	tok, err := getInstallationToken(jwtToken, owner, repo)
	if err != nil {
		return "", fmt.Errorf("GitHub adapter: failed to get installation token: %w", err)
	}
	return tok, nil
}

// ghPRResponse is the subset of the GitHub PR API response we care about.
type ghPRResponse struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (g *GitHubAdapter) GetPRDetails(owner, repo string, prNumber int) (*NormalizedPR, error) {
	tok, err := g.token(owner, repo)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	body, err := makeAuthenticatedRequest(tok, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("GitHub adapter: GetPRDetails request failed: %w", err)
	}

	var pr ghPRResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("GitHub adapter: failed to parse PR response: %w", err)
	}

	return &NormalizedPR{
		Number:       pr.Number,
		Title:        pr.Title,
		Description:  pr.Body,
		Author:       pr.User.Login,
		SourceBranch: pr.Head.Ref,
		TargetBranch: pr.Base.Ref,
		State:        pr.State,
		URL:          pr.HTMLURL,
	}, nil
}

func (g *GitHubAdapter) GetPRFiles(owner, repo string, prNumber int) ([]NormalizedFile, error) {
	tok, err := g.token(owner, repo)
	if err != nil {
		return nil, err
	}

	// Reuse the existing GitHub-specific fetcher from pullrequest.go.
	rawFiles, err := getPRChangedFiles(tok, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("GitHub adapter: GetPRFiles failed: %w", err)
	}

	files := make([]NormalizedFile, len(rawFiles))
	for i, f := range rawFiles {
		files[i] = NormalizedFile{
			Filename:         f.Filename,
			Status:           f.Status,
			Additions:        f.Additions,
			Deletions:        f.Deletions,
			Changes:          f.Changes,
			PreviousFilename: f.PreviousFilename,
		}
	}
	return files, nil
}

// ghWebhookPayload is the GitHub-specific webhook JSON structure.
type ghWebhookPayload struct {
	Action string `json:"action"`
	Number int    `json:"number"`

	PullRequest struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct{ Ref string `json:"ref"` } `json:"head"`
		Base struct{ Ref string `json:"ref"` } `json:"base"`
	} `json:"pull_request"`

	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		CloneURL string `json:"clone_url"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// NormalizeEvent parses the raw GitHub webhook payload, maps it to a
// NormalizedEvent, and enriches it with changed files for actionable PR events.
func (g *GitHubAdapter) NormalizeEvent(eventType string, payload []byte) (*NormalizedEvent, error) {
	var p ghWebhookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("GitHub adapter: failed to parse webhook payload: %w", err)
	}

	pr := p.PullRequest
	repo := p.Repository

	event := &NormalizedEvent{
		Platform:  PlatformGitHub,
		EventType: fmt.Sprintf("pull_request.%s", p.Action), // e.g. "pull_request.opened"
		Action:    p.Action,
		PR: NormalizedPR{
			Number:       pr.Number,
			Title:        pr.Title,
			Description:  pr.Body,
			Author:       pr.User.Login,
			SourceBranch: pr.Head.Ref,
			TargetBranch: pr.Base.Ref,
			State:        pr.State,
			URL:          pr.HTMLURL,
		},
		Repository: NormalizedRepository{
			Name:     repo.Name,
			FullName: repo.FullName,
			Owner:    repo.Owner.Login,
			CloneURL: repo.CloneURL,
			HTMLURL:  repo.HTMLURL,
		},
		RawPayload: payload,
		ReceivedAt: time.Now(),
	}

	// Fetch changed files for events that mutate the PR's commit set.
	if pr.Number != 0 && isFileEnrichableAction(p.Action) {
		log.Printf("[GitHub Adapter] Fetching files for PR #%d in %s\n", pr.Number, repo.FullName)
		files, err := g.GetPRFiles(repo.Owner.Login, repo.Name, pr.Number)
		if err != nil {
			log.Printf("[GitHub Adapter] Warning: could not fetch PR files: %v\n", err)
		} else {
			event.Files = files
		}
	}

	return event, nil
}

// isFileEnrichableAction returns true for PR actions where fetching changed
// files makes sense (opened, synchronize, reopened).
func isFileEnrichableAction(action string) bool {
	switch action {
	case "opened", "synchronize", "reopened":
		return true
	}
	return false
}
