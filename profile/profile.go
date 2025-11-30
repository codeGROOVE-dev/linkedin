// Package profile defines the common types for social media profile extraction.
package profile

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by platform packages.
var (
	ErrAuthRequired    = errors.New("authentication required")
	ErrNoCookies       = errors.New("no cookies available")
	ErrProfileNotFound = errors.New("profile not found")
	ErrRateLimited     = errors.New("rate limited")
)

// Profile represents extracted data from a social media profile.
//
//nolint:govet // fieldalignment: intentional layout for readability
type Profile struct {
	// Metadata
	Platform      string `json:",omitempty"` // Platform name: "linkedin", "twitter", "mastodon", etc.
	URL           string `json:",omitempty"` // Original URL fetched
	Authenticated bool   `json:",omitempty"` // Whether login cookies were used

	// Core profile data
	Username string `json:",omitempty"` // Handle/username (without @ prefix)
	Name     string `json:",omitempty"` // Display name
	Bio      string `json:",omitempty"` // Profile bio/description
	Location string `json:",omitempty"` // Geographic location
	Website  string `json:",omitempty"` // Personal website URL

	// Platform-specific fields
	Fields map[string]string `json:",omitempty"` // Additional platform-specific data (headline, employer, etc.)

	// For further crawling
	SocialLinks []string `json:",omitempty"` // Other social media URLs detected on the profile

	// Fallback for unrecognized platforms
	Unstructured string `json:",omitempty"` // Raw markdown content (HTML->MD conversion)

	// Guess mode fields (omitted from JSON when empty)
	IsGuess    bool     `json:",omitempty"` // True if this profile was discovered via guessing
	Confidence float64  `json:",omitempty"` // Confidence score 0.0-1.0 for guessed profiles
	GuessMatch []string `json:",omitempty"` // Reasons for match (e.g., "username", "name", "location")
}

// HTTPCache defines the interface for caching HTTP responses.
// This is compatible with locator's httpcache package.
type HTTPCache interface {
	Get(ctx context.Context, url string) (data []byte, etag string, headers map[string]string, found bool)
	SetAsync(ctx context.Context, url string, data []byte, etag string, headers map[string]string) error
	SetAsyncWithTTL(ctx context.Context, url string, data []byte, etag string, headers map[string]string, ttl time.Duration) error
}
