// Package github fetches GitHub profile data.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/sociopath/pkg/cache"
	"github.com/codeGROOVE-dev/sociopath/pkg/htmlutil"
	"github.com/codeGROOVE-dev/sociopath/pkg/profile"
)

const platform = "github"

// Match returns true if the URL is a GitHub profile URL.
func Match(urlStr string) bool {
	lower := strings.ToLower(urlStr)
	if !strings.Contains(lower, "github.com/") {
		return false
	}
	// Extract path after github.com/
	idx := strings.Index(lower, "github.com/")
	path := lower[idx+len("github.com/"):]
	path = strings.TrimSuffix(path, "/")
	if qIdx := strings.Index(path, "?"); qIdx >= 0 {
		path = path[:qIdx]
	}
	// Must be just username (no slashes)
	if strings.Contains(path, "/") {
		return false
	}
	// Skip known non-profile paths
	nonProfiles := map[string]bool{
		"features": true, "security": true, "enterprise": true, "team": true,
		"marketplace": true, "sponsors": true, "topics": true, "trending": true,
		"collections": true, "orgs": true, "solutions": true, "resources": true,
		"customer-stories": true, "partners": true, "accelerator": true,
		"trust-center": true, "why-github": true, "mcp": true, "fluidicon": true,
		"login": true, "join": true, "pricing": true, "about": true,
		"premium-support": true, "newsletter": true, "edu": true, "mobile": true,
		"readme": true, "explore": true, "new": true, "settings": true,
		"notifications": true, "issues": true, "pulls": true, "codespaces": true,
		"copilot": true, "actions": true, "projects": true, "packages": true,
		"discussions": true, "wiki": true, "stars": true, "watching": true,
		"search": true, "site": true, "apps": true,
	}
	return path != "" && !nonProfiles[path]
}

// AuthRequired returns false because GitHub profiles are public.
func AuthRequired() bool { return false }

// Client handles GitHub requests.
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

// New creates a GitHub client.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	cfg := &config{logger: slog.Default()}
	for _, opt := range opts {
		opt(cfg)
	}

	return &Client{
		httpClient: &http.Client{Timeout: 3 * time.Second},
		cache:      cfg.cache,
		logger:     cfg.logger,
	}, nil
}

// Fetch retrieves a GitHub profile.
func (c *Client) Fetch(ctx context.Context, urlStr string) (*profile.Profile, error) {
	username := extractUsername(urlStr)
	if username == "" {
		return nil, fmt.Errorf("could not extract username from: %s", urlStr)
	}

	// Normalize URL
	if !strings.HasPrefix(urlStr, "http") {
		urlStr = "https://github.com/" + username
	}

	c.logger.InfoContext(ctx, "fetching github profile", "url", urlStr, "username", username)

	// Fetch API data
	prof, err := c.fetchAPI(ctx, urlStr, username)
	if err != nil {
		return nil, err
	}

	// Fetch HTML to extract rel="me" links, README, and organizations
	htmlContent, htmlLinks := c.fetchHTML(ctx, urlStr)
	prof.SocialLinks = append(prof.SocialLinks, htmlLinks...)

	// Extract README and organizations from HTML if available
	if htmlContent != "" {
		// Extract organizations
		orgs := extractOrganizations(htmlContent)
		if len(orgs) > 0 {
			prof.Fields["organizations"] = strings.Join(orgs, ", ")
		}

		// Extract README - get raw HTML for link extraction, then convert to markdown
		readmeHTML := extractREADMEHTML(htmlContent)
		if readmeHTML != "" {
			// Extract social links from raw HTML (before conversion loses image-only links)
			readmeLinks := htmlutil.SocialLinks(readmeHTML)
			prof.SocialLinks = append(prof.SocialLinks, readmeLinks...)

			// Convert to markdown for unstructured content
			prof.Unstructured = htmlutil.ToMarkdown(readmeHTML)
		}
	}

	// Deduplicate and filter out same-platform links (GitHub to GitHub)
	prof.SocialLinks = dedupeLinks(prof.SocialLinks)
	prof.SocialLinks = filterSamePlatformLinks(prof.SocialLinks)

	return prof, nil
}

func (c *Client) fetchAPI(ctx context.Context, urlStr, username string) (*profile.Profile, error) {
	apiURL := "https://api.github.com/users/" + username

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "sociopath/1.0")

	body, err := cache.FetchURL(ctx, c.cache, c.httpClient, req, c.logger)
	if err != nil {
		return nil, err
	}

	return parseJSON(body, urlStr, username)
}

func (c *Client) fetchHTML(ctx context.Context, urlStr string) (content string, links []string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
	if err != nil {
		c.logger.Debug("failed to create HTML request", "error", err)
		return "", nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:146.0) Gecko/20100101 Firefox/146.0")

	body, err := cache.FetchURL(ctx, c.cache, c.httpClient, req, c.logger)
	if err != nil {
		c.logger.Debug("failed to fetch HTML", "error", err)
		return "", nil
	}

	content = string(body)
	links = extractSocialLinks(content)
	return content, links
}

// extractREADMEHTML extracts the raw README HTML from GitHub profile page.
func extractREADMEHTML(htmlContent string) string {
	// GitHub embeds README in <article class="markdown-body entry-content ...">
	// Extract everything from the opening tag to the closing </article>
	articlePattern := regexp.MustCompile(`(?s)<article[^>]*class="[^"]*markdown-body[^"]*"[^>]*>(.*?)</article>`)
	matches := articlePattern.FindStringSubmatch(htmlContent)
	if len(matches) < 2 {
		return ""
	}

	readmeHTML := matches[1]
	if strings.TrimSpace(readmeHTML) == "" {
		return ""
	}

	return readmeHTML
}

// extractSocialLinks extracts social media links from HTML, focusing on rel="me" verified links.
func extractSocialLinks(html string) []string {
	var links []string

	// GitHub uses rel="nofollow me" for verified social links
	// Example: <a rel="nofollow me" href="https://triangletoot.party/@thomrstrom">...</a>
	relMePattern := regexp.MustCompile(`<a[^>]+rel=["'][^"']*\bme\b[^"']*["'][^>]+href=["']([^"']+)["']`)
	matches := relMePattern.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		if len(match) > 1 {
			link := match[1]
			// Filter out GitHub URLs and email URLs
			if !strings.Contains(link, "github.com") && !htmlutil.IsEmailURL(link) {
				links = append(links, link)
			}
		}
	}

	// Also check for href first, then rel (both orders work)
	hrefFirstPattern := regexp.MustCompile(`<a[^>]+href=["']([^"']+)["'][^>]+rel=["'][^"']*\bme\b[^"']*["']`)
	matches = hrefFirstPattern.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}
		link := match[1]
		// Skip GitHub links, email URLs, and duplicates
		if strings.Contains(link, "github.com") || htmlutil.IsEmailURL(link) {
			continue
		}
		isDuplicate := false
		for _, existing := range links {
			if existing == link {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			links = append(links, link)
		}
	}

	return links
}

// extractOrganizations extracts organization names from GitHub profile HTML.
// Organizations are listed in the profile sidebar with aria-label attributes.
func extractOrganizations(html string) []string {
	// Pattern: aria-label="organizationname"
	// This matches the organization links in the profile sidebar
	pattern := regexp.MustCompile(`aria-label="([^"]+)"[^>]*>\s*<img[^>]+alt="@([^"]+)"`)
	matches := pattern.FindAllStringSubmatch(html, -1)

	var orgs []string
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 2 {
			orgName := match[1]
			// Skip if already seen
			if seen[orgName] {
				continue
			}
			seen[orgName] = true
			orgs = append(orgs, orgName)
		}
	}

	// Fallback pattern: just look for organization links
	if len(orgs) == 0 {
		linkPattern := regexp.MustCompile(`href="/([^/"]+)"[^>]*aria-label="([^"]+)"`)
		matches = linkPattern.FindAllStringSubmatch(html, -1)
		for _, match := range matches {
			if len(match) > 2 {
				orgName := match[2]
				// Filter out obviously non-org labels
				if strings.Contains(strings.ToLower(orgName), "organization") ||
					len(orgName) < 50 && !strings.Contains(orgName, " ") {
					if !seen[orgName] {
						seen[orgName] = true
						orgs = append(orgs, orgName)
					}
				}
			}
		}
	}

	return orgs
}

func parseJSON(data []byte, urlStr, _ string) (*profile.Profile, error) {
	//nolint:govet // fieldalignment: intentional layout for readability
	var ghUser struct {
		Login       string `json:"login"`
		Name        string `json:"name"`
		Bio         string `json:"bio"`
		Location    string `json:"location"`
		Blog        string `json:"blog"`
		Email       string `json:"email"`
		TwitterUser string `json:"twitter_username"`
		Company     string `json:"company"`
		PublicRepos int    `json:"public_repos"`
		Followers   int    `json:"followers"`
		Following   int    `json:"following"`
		AvatarURL   string `json:"avatar_url"`
		HTMLURL     string `json:"html_url"`
		Type        string `json:"type"`
	}

	if err := json.Unmarshal(data, &ghUser); err != nil {
		return nil, err
	}

	prof := &profile.Profile{
		Platform:      platform,
		URL:           urlStr,
		Authenticated: false,
		Username:      ghUser.Login,
		Name:          ghUser.Name,
		Bio:           ghUser.Bio,
		Location:      ghUser.Location,
		Fields:        make(map[string]string),
	}

	// Add website or email
	if ghUser.Blog != "" {
		blog := ghUser.Blog
		blogLower := strings.ToLower(blog)

		// Check for mailto: links first
		if strings.HasPrefix(blogLower, "mailto:") {
			email := strings.TrimPrefix(blogLower, "mailto:")
			prof.Fields["email"] = email
		} else {
			// GitHub sometimes stores URLs without protocol
			website := blog
			if !strings.HasPrefix(website, "http") {
				website = "https://" + website
			}

			// Check if this is actually an email address with http(s):// prefix
			if email, isEmail := htmlutil.ExtractEmailFromURL(website); isEmail {
				prof.Fields["email"] = email
			} else {
				prof.Website = website
				prof.Fields["website"] = website
				// Don't add to SocialLinks - it's already in prof.Website which is followed by recursive mode
			}
		}
	}

	// Add email
	if ghUser.Email != "" {
		prof.Fields["email"] = ghUser.Email
	}

	// Add company
	if ghUser.Company != "" {
		// Remove @ prefix if present
		company := strings.TrimPrefix(ghUser.Company, "@")
		prof.Fields["company"] = company
	}

	// Add Twitter username
	if ghUser.TwitterUser != "" {
		twitterURL := "https://twitter.com/" + ghUser.TwitterUser
		prof.Fields["twitter"] = twitterURL
		prof.SocialLinks = append(prof.SocialLinks, twitterURL)
	}

	// Add stats
	if ghUser.PublicRepos > 0 {
		prof.Fields["public_repos"] = strconv.Itoa(ghUser.PublicRepos)
	}
	if ghUser.Followers > 0 {
		prof.Fields["followers"] = strconv.Itoa(ghUser.Followers)
	}
	if ghUser.Following > 0 {
		prof.Fields["following"] = strconv.Itoa(ghUser.Following)
	}

	// Add avatar URL
	if ghUser.AvatarURL != "" {
		prof.Fields["avatar_url"] = ghUser.AvatarURL
	}

	// Add account type
	if ghUser.Type != "" {
		prof.Fields["type"] = ghUser.Type
	}

	return prof, nil
}

func extractUsername(urlStr string) string {
	// Remove protocol
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = strings.TrimPrefix(urlStr, "http://")
	urlStr = strings.TrimPrefix(urlStr, "www.")

	// Extract github.com/username
	re := regexp.MustCompile(`github\.com/([^/?]+)`)
	if matches := re.FindStringSubmatch(urlStr); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func filterSamePlatformLinks(links []string) []string {
	var filtered []string
	for _, link := range links {
		// Skip GitHub URLs
		if !Match(link) {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func dedupeLinks(links []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, link := range links {
		normalized := strings.TrimSuffix(strings.ToLower(link), "/")
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, link)
		}
	}
	return result
}
