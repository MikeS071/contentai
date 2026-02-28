package content

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrNotFound    = errors.New("content item not found")
	errInvalidSlug = errors.New("invalid slug")
)

var slugPattern = regexp.MustCompile(`^[a-z0-9-]{3,100}$`)

type Status string

const (
	StatusDraft           Status = "draft"
	StatusQAPassed        Status = "qa_passed"
	StatusPublished       Status = "published"
	StatusSocialGenerated Status = "social_generated"
	StatusScheduled       Status = "scheduled"
	StatusPosted          Status = "posted"
)

type Meta struct {
	Title       string     `json:"title"`
	Slug        string     `json:"slug"`
	Summary     string     `json:"summary,omitempty"`
	Status      Status     `json:"status"`
	Category    string     `json:"category,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	PublishURL  string     `json:"publish_url,omitempty"`
}

type SocialJSON struct {
	XText        string `json:"xText"`
	LinkedInText string `json:"linkedinText"`
	XPostURL     string `json:"xPostUrl,omitempty"`
	LinkedInURL  string `json:"linkedinPostUrl,omitempty"`
}

type QAJSON struct {
	Checks []QACheck `json:"checks"`
	Passed bool      `json:"passed"`
	RunAt  time.Time `json:"run_at"`
}

type QACheck struct {
	Name   string   `json:"name"`
	Passed bool     `json:"passed"`
	Issues []string `json:"issues,omitempty"`
	Fixes  []QAFix  `json:"fixes,omitempty"`
}

type QAFix struct {
	Original string `json:"original"`
	Fixed    string `json:"fixed"`
	Applied  bool   `json:"applied"`
}

func ValidateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("%w: must match %q", errInvalidSlug, slugPattern.String())
	}
	return nil
}

func IsValidStatus(status Status) bool {
	switch status {
	case StatusDraft, StatusQAPassed, StatusPublished, StatusSocialGenerated, StatusScheduled, StatusPosted:
		return true
	default:
		return false
	}
}

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	if !IsValidStatus(status) {
		return "", fmt.Errorf("invalid status %q", value)
	}
	return status, nil
}
