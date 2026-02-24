package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// BitbucketAdapter implements SCMAdapter for Bitbucket Cloud.
//
// Authentication uses Bitbucket App Passwords (HTTP Basic Auth).
// Required env vars: BITBUCKET_USERNAME, BITBUCKET_APP_PASSWORD.
//
// Relevant Bitbucket API v2 endpoints used:
//   GET  /2.0/repositories/{workspace}/{repo}/pullrequests/{id}
//   GET  /2.0/repositories/{workspace}/{repo}/pullrequests/{id}/diffstat
type BitbucketAdapter struct {
	username    string
	appPassword string
	baseURL     string
}

// NewBitbucketAdapter creates a BitbucketAdapter from environment credentials.
func NewBitbucketAdapter() (*BitbucketAdapter, error) {
	username := os.Getenv("BITBUCKET_USERNAME")
	appPassword := os.Getenv("BITBUCKET_APP_PASSWORD")
	if username == "" || appPassword == "" {
		return nil, fmt.Errorf("Bitbucket adapter: BITBUCKET_USERNAME and BITBUCKET_APP_PASSWORD must be set")
	}
	return &BitbucketAdapter{
		username:    username,
		appPassword: appPassword,
		baseURL:     "https://api.bitbucket.org/2.0",
	}, nil
}

func (b *BitbucketAdapter) Platform() SCMPlatform {
	return PlatformBitbucket
}

// request makes an authenticated GET request to the Bitbucket API.
func (b *BitbucketAdapter) request(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(b.username, b.appPassword)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Bitbucket API %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// bbPRResponse is the subset of the Bitbucket PR API response we care about.
type bbPRResponse struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	Author      struct {
		Nickname    string `json:"nickname"`
		DisplayName string `json:"display_name"`
	} `json:"author"`
	Source struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"source"`
	Destination struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"destination"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

func (b *BitbucketAdapter) GetPRDetails(owner, repo string, prNumber int) (*NormalizedPR, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d", b.baseURL, owner, repo, prNumber)
	body, err := b.request(url)
	if err != nil {
		return nil, fmt.Errorf("Bitbucket adapter: GetPRDetails failed: %w", err)
	}

	var pr bbPRResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("Bitbucket adapter: failed to parse PR response: %w", err)
	}

	return &NormalizedPR{
		Number:       pr.ID,
		Title:        pr.Title,
		Description:  pr.Description,
		Author:       pr.Author.Nickname,
		SourceBranch: pr.Source.Branch.Name,
		TargetBranch: pr.Destination.Branch.Name,
		State:        strings.ToLower(pr.State),
		URL:          pr.Links.HTML.Href,
	}, nil
}

// bbDiffstatResponse is the Bitbucket diffstat API response structure.
type bbDiffstatResponse struct {
	Values []struct {
		Status       string `json:"status"` // "added", "removed", "modified", "renamed"
		LinesAdded   int    `json:"lines_added"`
		LinesRemoved int    `json:"lines_removed"`
		New          *struct {
			Path string `json:"path"`
		} `json:"new"`
		Old *struct {
			Path string `json:"path"`
		} `json:"old"`
	} `json:"values"`
}

func (b *BitbucketAdapter) GetPRFiles(owner, repo string, prNumber int) ([]NormalizedFile, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/diffstat", b.baseURL, owner, repo, prNumber)
	body, err := b.request(url)
	if err != nil {
		return nil, fmt.Errorf("Bitbucket adapter: GetPRFiles failed: %w", err)
	}

	var diffstat bbDiffstatResponse
	if err := json.Unmarshal(body, &diffstat); err != nil {
		return nil, fmt.Errorf("Bitbucket adapter: failed to parse diffstat response: %w", err)
	}

	files := make([]NormalizedFile, 0, len(diffstat.Values))
	for _, v := range diffstat.Values {
		f := NormalizedFile{
			Status:    mapBitbucketStatus(v.Status),
			Additions: v.LinesAdded,
			Deletions: v.LinesRemoved,
			Changes:   v.LinesAdded + v.LinesRemoved,
		}
		if v.New != nil {
			f.Filename = v.New.Path
		}
		if v.Old != nil && strings.ToLower(v.Status) == "renamed" {
			f.PreviousFilename = v.Old.Path
		}
		files = append(files, f)
	}
	return files, nil
}

// mapBitbucketStatus normalises Bitbucket file-change status strings to the
// common vocabulary shared across all adapters.
func mapBitbucketStatus(status string) string {
	switch strings.ToLower(status) {
	case "added":
		return "added"
	case "removed":
		return "removed"
	case "modified":
		return "modified"
	case "renamed":
		return "renamed"
	default:
		return "modified"
	}
}

// bbWebhookPayload is the Bitbucket-specific webhook JSON structure.
// Bitbucket sends a single object whose top-level key is either "pullrequest"
// or "repository" depending on the event type.
type bbWebhookPayload struct {
	PullRequest struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		Author      struct {
			Nickname    string `json:"nickname"`
			DisplayName string `json:"display_name"`
		} `json:"author"`
		Source struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"source"`
		Destination struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		} `json:"destination"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	} `json:"pullrequest"`

	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"` // "workspace/repo-slug"
		Owner    struct {
			Nickname    string `json:"nickname"`
			DisplayName string `json:"display_name"`
		} `json:"owner"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
			Clone []struct {
				Href string `json:"href"`
				Name string `json:"name"` // "https" or "ssh"
			} `json:"clone"`
		} `json:"links"`
	} `json:"repository"`
}

// mapBitbucketEventKey converts a Bitbucket X-Event-Key value into the
// normalised (eventType, action) pair used by NormalizedEvent.
func mapBitbucketEventKey(key string) (eventType, action string) {
	switch key {
	case "pullrequest:created":
		return "pull_request.opened", "opened"
	case "pullrequest:updated":
		return "pull_request.updated", "synchronize"
	case "pullrequest:fulfilled":
		return "pull_request.closed", "closed"
	case "pullrequest:rejected":
		return "pull_request.closed", "closed"
	default:
		return "pull_request.unknown", "unknown"
	}
}

// NormalizeEvent parses the raw Bitbucket webhook payload, maps it to a
// NormalizedEvent, and enriches it with changed files for actionable PR events.
func (b *BitbucketAdapter) NormalizeEvent(eventType string, payload []byte) (*NormalizedEvent, error) {
	var p bbWebhookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("Bitbucket adapter: failed to parse webhook payload: %w", err)
	}

	normalizedType, action := mapBitbucketEventKey(eventType)

	pr := p.PullRequest
	repo := p.Repository

	// Bitbucket full_name is "workspace/repo-slug"; split to get owner.
	parts := strings.SplitN(repo.FullName, "/", 2)
	owner, repoName := "", repo.Name
	if len(parts) == 2 {
		owner, repoName = parts[0], parts[1]
	}

	// Prefer HTTPS clone URL.
	cloneURL := ""
	for _, link := range repo.Links.Clone {
		if link.Name == "https" {
			cloneURL = link.Href
			break
		}
	}

	event := &NormalizedEvent{
		Platform:  PlatformBitbucket,
		EventType: normalizedType,
		Action:    action,
		PR: NormalizedPR{
			Number:       pr.ID,
			Title:        pr.Title,
			Description:  pr.Description,
			Author:       pr.Author.Nickname,
			SourceBranch: pr.Source.Branch.Name,
			TargetBranch: pr.Destination.Branch.Name,
			State:        strings.ToLower(pr.State),
			URL:          pr.Links.HTML.Href,
		},
		Repository: NormalizedRepository{
			Name:     repoName,
			FullName: repo.FullName,
			Owner:    owner,
			CloneURL: cloneURL,
			HTMLURL:  repo.Links.HTML.Href,
		},
		RawPayload: payload,
		ReceivedAt: time.Now(),
	}

	// Fetch changed files for opened / updated events.
	if pr.ID != 0 && (action == "opened" || action == "synchronize") {
		log.Printf("[Bitbucket Adapter] Fetching files for PR #%d in %s\n", pr.ID, repo.FullName)
		files, err := b.GetPRFiles(owner, repoName, pr.ID)
		if err != nil {
			log.Printf("[Bitbucket Adapter] Warning: could not fetch PR files: %v\n", err)
		} else {
			event.Files = files
		}
	}

	return event, nil
}
