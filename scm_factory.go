package main

import (
	"fmt"
	"net/http"
)

// DetectPlatform inspects the webhook HTTP headers to determine which SCM
// provider sent the request.
//
//   - GitHub sends:    X-GitHub-Event
//   - Bitbucket sends: X-Event-Key  (e.g. "pullrequest:created")
func DetectPlatform(headers http.Header) SCMPlatform {
	if headers.Get("X-GitHub-Event") != "" {
		return PlatformGitHub
	}
	if headers.Get("X-Event-Key") != "" {
		return PlatformBitbucket
	}
	return PlatformUnknown
}

// NewSCMAdapter returns the SCMAdapter implementation for the detected platform.
// Returns an error if the platform is unsupported or the adapter cannot be
// initialised (e.g. missing credentials).
func NewSCMAdapter(platform SCMPlatform) (SCMAdapter, error) {
	switch platform {
	case PlatformGitHub:
		return NewGitHubAdapter()
	case PlatformBitbucket:
		return NewBitbucketAdapter()
	default:
		return nil, fmt.Errorf("unsupported SCM platform: %q", platform)
	}
}
