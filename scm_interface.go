package main

import (
	"log"
	"time"
)

// SCMPlatform identifies a source code management provider.
type SCMPlatform string

const (
	PlatformGitHub    SCMPlatform = "github"
	PlatformBitbucket SCMPlatform = "bitbucket"
	PlatformUnknown   SCMPlatform = "unknown"
)

// NormalizedPR is a platform-agnostic pull request representation.
type NormalizedPR struct {
	Number       int
	Title        string
	Description  string
	Author       string
	SourceBranch string
	TargetBranch string
	State        string
	URL          string
}

// NormalizedRepository is a platform-agnostic repository representation.
type NormalizedRepository struct {
	Name     string
	FullName string
	Owner    string
	CloneURL string
	HTMLURL  string
}

// NormalizedFile is a platform-agnostic changed-file representation.
// Status values: "added", "modified", "removed", "renamed".
type NormalizedFile struct {
	Filename         string
	Status           string
	Additions        int
	Deletions        int
	Changes          int
	PreviousFilename string // only set when Status == "renamed"
}

// NormalizedEvent is the unified event the SCM Adapter emits after consuming a
// raw webhook, enriching it with PR metadata and changed files.
type NormalizedEvent struct {
	Platform   SCMPlatform
	EventType  string // e.g. "pull_request.opened", "pull_request.closed"
	Action     string // e.g. "opened", "synchronize", "closed"
	PR         NormalizedPR
	Repository NormalizedRepository
	Files      []NormalizedFile
	RawPayload []byte
	ReceivedAt time.Time
}

// SCMAdapter is the interface every SCM provider must implement.
// Adding support for a new SCM (GitLab, Azure DevOps, …) means creating a
// struct that satisfies this interface — no other code needs to change.
type SCMAdapter interface {
	// Platform returns the identifier of the SCM this adapter handles.
	Platform() SCMPlatform

	// GetPRDetails fetches pull-request metadata from the SCM API and returns
	// it in the normalized format.
	GetPRDetails(owner, repo string, prNumber int) (*NormalizedPR, error)

	// GetPRFiles fetches the list of files changed in a pull request and
	// returns them in the normalized format.
	GetPRFiles(owner, repo string, prNumber int) ([]NormalizedFile, error)

	// NormalizeEvent converts a raw webhook payload into a NormalizedEvent,
	// fetching additional PR details and file lists as needed.
	NormalizeEvent(eventType string, payload []byte) (*NormalizedEvent, error)
}

// logNormalizedEvent prints a structured summary of a NormalizedEvent.
func logNormalizedEvent(event *NormalizedEvent) {
	log.Println("=== Normalized SCM Event ===")
	log.Printf("  Platform:   %s\n", event.Platform)
	log.Printf("  Event Type: %s\n", event.EventType)
	log.Printf("  Action:     %s\n", event.Action)
	log.Printf("  PR:         #%d — %s\n", event.PR.Number, event.PR.Title)
	log.Printf("  Author:     %s\n", event.PR.Author)
	log.Printf("  Branches:   %s -> %s\n", event.PR.SourceBranch, event.PR.TargetBranch)
	log.Printf("  State:      %s\n", event.PR.State)
	log.Printf("  URL:        %s\n", event.PR.URL)
	log.Printf("  Repo:       %s (owner: %s)\n", event.Repository.FullName, event.Repository.Owner)
	log.Printf("  Files (%d changed):\n", len(event.Files))
	for _, f := range event.Files {
		if f.Status == "renamed" {
			log.Printf("    [%s] %s -> %s (+%d -%d)\n",
				f.Status, f.PreviousFilename, f.Filename, f.Additions, f.Deletions)
		} else {
			log.Printf("    [%s] %s (+%d -%d)\n",
				f.Status, f.Filename, f.Additions, f.Deletions)
		}
	}
}
