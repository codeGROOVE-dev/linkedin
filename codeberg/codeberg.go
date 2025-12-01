// Package codeberg fetches Codeberg profile data.
package codeberg

import (
	"context"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/sociopath/cache"
	"github.com/codeGROOVE-dev/sociopath/profile"
)

const platform = "codeberg"

// Match returns true if the URL is a Codeberg profile URL.
func Match(urlStr string) bool {
	lower := strings.ToLower(urlStr)
	if !strings.Contains(lower, "codeberg.org/") {
		return false
	}
	// Extract path after codeberg.org/
	idx := strings.Index(lower, "codeberg.org/")
	path := lower[idx+len("codeberg.org/"):]
	path = strings.TrimSuffix(path, "/")
	if qIdx := strings.Index(path, "?"); qIdx >= 0 {
		path = path[:qIdx]
	}
	// Must be just username (no slashes) for a profile page
	if strings.Contains(path, "/") {
		return false
	}
	// Skip known non-profile paths
	nonProfiles := map[string]bool{
		"explore": true, "user": true, "repo": true, "org": true,
		"admin": true, "api": true, "swagger": true, "assets": true,
		"codeberg": true, "codeberg-infrastructure": true,
	}
	return path != "" && !nonProfiles[path]
}

// AuthRequired returns false because Codeberg profiles are public.
func AuthRequired() bool { return false }

// Client handles Codeberg requests.
type Client struct {
	httpClient *http.Client
	cache      cache.HTTPCache
	logger     *slog.Logger
}

// Option configures a Client.
type Option func(*config)

type config struct {
	cache  cache.HTTPCache
	logger *slog.Logger
}

// WithHTTPCache sets the HTTP cache.
func WithHTTPCache(httpCache cache.HTTPCache) Option {
	return func(c *config) { c.cache = httpCache }
}

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) { c.logger = logger }
}

// New creates a Codeberg client.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	cfg := &config{logger: slog.Default()}
	for _, opt := range opts {
		opt(cfg)
	}

	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		cache:      cfg.cache,
		logger:     cfg.logger,
	}, nil
}

// Fetch retrieves a Codeberg profile.
func (c *Client) Fetch(ctx context.Context, urlStr string) (*profile.Profile, error) {
	username := extractUsername(urlStr)
	if username == "" {
		return nil, fmt.Errorf("could not extract username from: %s", urlStr)
	}

	// Normalize URL
	if !strings.HasPrefix(urlStr, "http") {
		urlStr = "https://codeberg.org/" + username
	}

	c.logger.InfoContext(ctx, "fetching codeberg profile", "url", urlStr, "username", username)

	// Check cache
	if c.cache != nil {
		if data, _, _, found := c.cache.Get(ctx, urlStr); found {
			return parseHTML(data, urlStr, username), nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:146.0) Gecko/20100101 Firefox/146.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // error ignored intentionally

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	// Cache response (async, errors intentionally ignored)
	if c.cache != nil {
		_ = c.cache.SetAsync(ctx, urlStr, body, "", nil) //nolint:errcheck // async, error ignored
	}

	return parseHTML(body, urlStr, username), nil
}

func parseHTML(data []byte, urlStr, username string) *profile.Profile {
	content := string(data)

	p := &profile.Profile{
		Platform:      platform,
		URL:           urlStr,
		Authenticated: false,
		Username:      username,
		Fields:        make(map[string]string),
	}

	// Extract name from og:title meta tag or title attribute on avatar
	// Pattern: <meta property="og:title" content="Woohyun Joh">
	ogTitlePattern := regexp.MustCompile(`<meta\s+property="og:title"\s+content="([^"]+)"`)
	if m := ogTitlePattern.FindStringSubmatch(content); len(m) > 1 {
		p.Name = strings.TrimSpace(html.UnescapeString(m[1]))
	}

	// Fallback: Extract from avatar title attribute
	// Pattern: title="Woohyun Joh"
	if p.Name == "" {
		avatarTitlePattern := regexp.MustCompile(`<img[^>]+class="[^"]*avatar[^"]*"[^>]+title="([^"]+)"`)
		if m := avatarTitlePattern.FindStringSubmatch(content); len(m) > 1 {
			p.Name = strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}

	// Fallback: Extract from profile-avatar-name header
	// Pattern: <span class="header text center">Woohyun Joh</span>
	if p.Name == "" {
		headerPattern := regexp.MustCompile(`<span\s+class="header[^"]*"[^>]*>([^<]+)</span>`)
		if m := headerPattern.FindStringSubmatch(content); len(m) > 1 {
			p.Name = strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}

	// Extract bio/description from og:description meta tag
	// This contains the user's bio, not Codeberg's default description
	// Pattern: <meta property="og:description" content="...">
	ogDescPattern := regexp.MustCompile(`<meta\s+property="og:description"\s+content="([^"]+)"`)
	if m := ogDescPattern.FindStringSubmatch(content); len(m) > 1 {
		bio := strings.TrimSpace(html.UnescapeString(m[1]))
		// Filter out Codeberg's default description
		if bio != "" && !strings.Contains(bio, "Codeberg is a non-profit") {
			p.Bio = bio
		}
	}

	// Extract website if present (users can add a website link)
	// Look for links with rel="...me..." which indicates a verified personal link
	// Pattern: <a ... rel="noopener noreferrer me" href="https://...">https://...</a>
	websitePattern := regexp.MustCompile(`<a[^>]+rel="[^"]*\bme\b[^"]*"[^>]+href="(https?://[^"]+)"`)
	if m := websitePattern.FindStringSubmatch(content); len(m) > 1 {
		website := m[1]
		// Filter out Codeberg's own links
		if !strings.Contains(website, "codeberg.org") &&
			!strings.Contains(website, "docs.codeberg.org") &&
			!strings.Contains(website, "blog.codeberg.org") {
			p.Website = website
		}
	}
	// Also try href first pattern
	if p.Website == "" {
		websitePattern2 := regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"[^>]+rel="[^"]*\bme\b[^"]*"`)
		if m := websitePattern2.FindStringSubmatch(content); len(m) > 1 {
			website := m[1]
			if !strings.Contains(website, "codeberg.org") {
				p.Website = website
			}
		}
	}

	// Extract join date
	// Pattern: Joined on 2023-04-06
	joinedPattern := regexp.MustCompile(`Joined\s+on\s+(\d{4}-\d{2}-\d{2})`)
	if m := joinedPattern.FindStringSubmatch(content); len(m) > 1 {
		p.Fields["joined"] = m[1]
	}

	// Extract follower/following counts
	followersPattern := regexp.MustCompile(`(\d+)\s*followers`)
	if m := followersPattern.FindStringSubmatch(content); len(m) > 1 {
		p.Fields["followers"] = m[1]
	}
	followingPattern := regexp.MustCompile(`(\d+)\s*following`)
	if m := followingPattern.FindStringSubmatch(content); len(m) > 1 {
		p.Fields["following"] = m[1]
	}

	// Extract pronouns if present (e.g., "he/him")
	// Pattern: johwhj  · he/him
	pronounsPattern := regexp.MustCompile(`class="username"[^>]*>[^<]*·\s*([^<]+)</span>`)
	if m := pronounsPattern.FindStringSubmatch(content); len(m) > 1 {
		pronouns := strings.TrimSpace(m[1])
		if pronouns != "" && len(pronouns) < 20 { // Sanity check
			p.Fields["pronouns"] = pronouns
		}
	}

	// Note: We intentionally do NOT extract social links from Codeberg pages
	// because the footer contains Codeberg's own institutional links (their Mastodon, blog, etc.)
	// which are not related to the user being profiled.

	return p
}

func extractUsername(urlStr string) string {
	// Remove protocol
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = strings.TrimPrefix(urlStr, "http://")
	urlStr = strings.TrimPrefix(urlStr, "www.")

	// Extract codeberg.org/username
	re := regexp.MustCompile(`codeberg\.org/([^/?]+)`)
	if matches := re.FindStringSubmatch(urlStr); len(matches) > 1 {
		return matches[1]
	}

	return ""
}
