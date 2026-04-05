package sync

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrRepoURLRequired    = errors.New("repo_url is required")
	ErrInvalidProvider    = errors.New("provider must be one of github, gitlab, bitbucket")
	ErrInvalidEventType   = errors.New("event_type must be one of repo.created, repo.updated, repo.deleted")
	ErrRequestedByMissing = errors.New("requested_by is required")
)

var allowedProviders = map[string]struct{}{
	"github":    {},
	"gitlab":    {},
	"bitbucket": {},
}

var allowedEventTypes = map[string]struct{}{
	"repo.created": {},
	"repo.updated": {},
	"repo.deleted": {},
}

type RepoEventRequest struct {
	RepoURL     string `json:"repo_url"`
	Provider    string `json:"provider"`
	Branch      string `json:"branch"`
	CommitSHA   string `json:"commit_sha"`
	EventType   string `json:"event_type"`
	RequestedBy string `json:"requested_by"`
}

type RepoEvent struct {
	EventID string `json:"event_id"`
	// Status      string    `json:"status"`
	RepoURL     string `json:"repo_url"`
	Provider    string `json:"provider"`
	Branch      string `json:"branch"`
	CommitSHA   string `json:"commit_sha"`
	EventType   string `json:"event_type"`
	RequestedBy string `json:"requested_by"`
	// Topic       string    `json:"topic"`
	ReceivedAt time.Time `json:"received_at"`
}

func (r RepoEventRequest) Validate() error {
	if strings.TrimSpace(r.RepoURL) == "" {
		return ErrRepoURLRequired
	}

	if _, ok := allowedProviders[strings.TrimSpace(r.Provider)]; !ok {
		return ErrInvalidProvider
	}

	if _, ok := allowedEventTypes[strings.TrimSpace(r.EventType)]; !ok {
		return ErrInvalidEventType
	}

	if strings.TrimSpace(r.RequestedBy) == "" {
		return ErrRequestedByMissing
	}

	return nil
}
