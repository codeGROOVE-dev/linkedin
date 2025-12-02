// Package generic provides HTML fallback extraction for unknown social media platforms.
package generic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/sociopath/pkg/cache"
	"github.com/codeGROOVE-dev/sociopath/pkg/htmlutil"
	"github.com/codeGROOVE-dev/sociopath/pkg/profile"
)

const (
	platform     = "generic"
	maxBlogPosts = 10
)

// Match always returns true as this is the fallback.
func Match(_ string) bool { return true }

// AuthRequired returns false.
func AuthRequired() bool { return false }

// Client handles generic website requests.
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

// New creates a generic client.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	cfg := &config{logger: slog.Default()}
	for _, opt := range opts {
		opt(cfg)
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // needed for corporate proxies
			},
		},
		cache:  cfg.cache,
		logger: cfg.logger,
	}, nil
}

// Fetch retrieves content from a generic website and converts to markdown.
func (c *Client) Fetch(ctx context.Context, urlStr string) (*profile.Profile, error) {
	// Normalize URL
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	// Security: validate URL
	if err := validateURL(urlStr); err != nil {
		return nil, err
	}

	c.logger.InfoContext(ctx, "fetching generic website", "url", urlStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:146.0) Gecko/20100101 Firefox/146.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	body, err := cache.FetchURL(ctx, c.cache, c.httpClient, req, c.logger)
	if err != nil {
		return nil, err
	}

	return parseHTML(body, urlStr), nil
}

func parseHTML(data []byte, urlStr string) *profile.Profile {
	content := string(data)

	p := &profile.Profile{
		Platform:      platform,
		URL:           urlStr,
		Authenticated: false,
		Fields:        make(map[string]string),
	}

	p.Name = htmlutil.Title(content)
	p.Bio = htmlutil.Description(content)
	p.Unstructured = htmlutil.ToMarkdown(content)

	// Extract social links
	p.SocialLinks = htmlutil.SocialLinks(content)

	// Also extract contact/about page links for recursion
	contactLinks := htmlutil.ContactLinks(content, urlStr)
	p.SocialLinks = append(p.SocialLinks, contactLinks...)

	// Deduplicate social links
	p.SocialLinks = dedupeLinks(p.SocialLinks)

	// Extract emails
	emails := htmlutil.EmailAddresses(content)
	if len(emails) > 0 {
		p.Fields["email"] = cleanEmail(emails[0]) // Primary email
		if len(emails) > 1 {
			// Store additional emails
			for i, email := range emails[1:] {
				p.Fields[fmt.Sprintf("email_%d", i+2)] = cleanEmail(email)
			}
		}
	}

	// Extract blog posts if this looks like a blog
	if posts := extractBlogPosts(content, urlStr); len(posts) > 0 {
		p.Posts = posts
		p.Platform = "blog"
		if len(posts) > 0 && posts[0].URL != "" {
			p.LastActive = extractDateFromURL(posts[0].URL)
		}
	}

	return p
}

// extractBlogPosts detects if a page is a blog and extracts post entries.
func extractBlogPosts(content, baseURL string) []profile.Post {
	// Check for blog indicators
	if !isBlogPage(content) {
		return nil
	}

	var posts []profile.Post

	// Parse base URL for resolving relative links
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	// Pattern 1: Links with dates in format YYYY-MM-DD or similar near them
	// e.g., <a href="/posts/2025/...">Title</a> - 2025-07-07
	datePostPattern := regexp.MustCompile(`(?i)<a[^>]+href=["']([^"']+)["'][^>]*>([^<]+)</a>\s*[-–—]\s*(\d{4}-\d{2}-\d{2})`)
	for _, m := range datePostPattern.FindAllStringSubmatch(content, maxBlogPosts) {
		postURL := resolveURL(base, m[1])
		if !isPostURL(postURL) {
			continue
		}
		posts = append(posts, profile.Post{
			Type:  profile.PostTypeArticle,
			Title: html.UnescapeString(strings.TrimSpace(m[2])),
			URL:   postURL,
		})
	}

	// If we found posts with the date pattern, return them
	if len(posts) > 0 {
		return limitPosts(posts)
	}

	// Pattern 2: Links with date prefix in link text (e.g., "2023-04-25 – Title")
	datePrefixPattern := regexp.MustCompile(`<a[^>]+href=["']([^"']+)["'][^>]*>(\d{4}-\d{2}-\d{2})\s*[-–—]\s*([^<]+)</a>`)
	for _, m := range datePrefixPattern.FindAllStringSubmatch(content, maxBlogPosts) {
		postURL := resolveURL(base, m[1])
		if !isPostURL(postURL) {
			continue
		}
		posts = append(posts, profile.Post{
			Type:  profile.PostTypeArticle,
			Title: html.UnescapeString(strings.TrimSpace(m[3])),
			URL:   postURL,
		})
	}

	if len(posts) > 0 {
		return limitPosts(posts)
	}

	// Pattern 3: All links within an <article> element pointing to post URLs
	articlePattern := regexp.MustCompile(`(?is)<article[^>]*>(.*?)</article>`)
	if m := articlePattern.FindStringSubmatch(content); len(m) > 1 {
		articleContent := m[1]
		linkPattern := regexp.MustCompile(`<a[^>]+href=["']([^"']+)["'][^>]*>([^<]+)</a>`)
		for _, lm := range linkPattern.FindAllStringSubmatch(articleContent, maxBlogPosts) {
			postURL := resolveURL(base, lm[1])
			if !isPostURL(postURL) {
				continue
			}
			posts = append(posts, profile.Post{
				Type:  profile.PostTypeArticle,
				Title: html.UnescapeString(strings.TrimSpace(lm[2])),
				URL:   postURL,
			})
		}
	}

	if len(posts) > 0 {
		return limitPosts(posts)
	}

	// Pattern 4: Look for links in post/blog sections
	// Find section with "posts", "articles", "blog" heading, then extract links
	sectionPattern := regexp.MustCompile(`(?is)<h[123][^>]*>[^<]*(?:posts?|articles?|blog)[^<]*</h[123]>\s*(.*?)(?:<h[123]|</body|$)`)
	if m := sectionPattern.FindStringSubmatch(content); len(m) > 1 {
		sectionContent := m[1]
		linkPattern := regexp.MustCompile(`<a[^>]+href=["']([^"']+)["'][^>]*>([^<]+)</a>`)
		for _, lm := range linkPattern.FindAllStringSubmatch(sectionContent, maxBlogPosts) {
			postURL := resolveURL(base, lm[1])
			if !isPostURL(postURL) {
				continue
			}
			posts = append(posts, profile.Post{
				Type:  profile.PostTypeArticle,
				Title: html.UnescapeString(strings.TrimSpace(lm[2])),
				URL:   postURL,
			})
		}
	}

	return limitPosts(posts)
}

// limitPosts returns at most maxBlogPosts posts.
func limitPosts(posts []profile.Post) []profile.Post {
	if len(posts) > maxBlogPosts {
		return posts[:maxBlogPosts]
	}
	return posts
}

// isBlogPage checks if the page appears to be a blog.
func isBlogPage(content string) bool {
	lower := strings.ToLower(content)

	// Check for RSS/Atom feed links (strong signal)
	if strings.Contains(lower, "application/rss+xml") || strings.Contains(lower, "application/atom+xml") {
		return true
	}

	// Check for blog-related URL patterns in links
	blogURLPatterns := []string{"/posts/", "/post/", "/blog/", "/articles/", "/article/"}
	linkCount := 0
	for _, pattern := range blogURLPatterns {
		linkCount += strings.Count(lower, pattern)
	}
	if linkCount >= 3 {
		return true
	}

	// Check for blog-related headings
	headingPattern := regexp.MustCompile(`(?i)<h[123][^>]*>[^<]*(?:recent posts?|latest posts?|blog posts?|articles?)[^<]*</h[123]>`)
	return headingPattern.MatchString(content)
}

// isPostURL checks if a URL looks like a blog post URL.
func isPostURL(urlStr string) bool {
	lower := strings.ToLower(urlStr)

	// Must contain blog-like path segments
	blogPaths := []string{"/posts/", "/post/", "/blog/", "/article/", "/articles/", "/news/", "/story/"}
	for _, path := range blogPaths {
		if strings.Contains(lower, path) {
			return true
		}
	}

	// Check for year patterns like /2024/ or /2025/
	yearPattern := regexp.MustCompile(`/20[12]\d/`)
	return yearPattern.MatchString(urlStr)
}

// resolveURL resolves a potentially relative URL against a base.
func resolveURL(base *url.URL, ref string) string {
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(refURL).String()
}

// extractDateFromURL extracts an ISO date from a URL containing year/month/day patterns.
func extractDateFromURL(urlStr string) string {
	// Look for /YYYY/MM/DD/ or /YYYY-MM-DD/ patterns
	datePattern := regexp.MustCompile(`/(20[12]\d)[/-]?(\d{2})?[/-]?(\d{2})?/`)
	if m := datePattern.FindStringSubmatch(urlStr); len(m) > 1 {
		year := m[1]
		month := "01"
		day := "01"
		if len(m) > 2 && m[2] != "" {
			month = m[2]
		}
		if len(m) > 3 && m[3] != "" {
			day = m[3]
		}
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return ""
}

// cleanEmail removes anti-spam text from email addresses.
func cleanEmail(email string) string {
	// Remove "NOSPAM" (case-insensitive) from email addresses
	lower := strings.ToLower(email)
	if strings.Contains(lower, "nospam") {
		// Find position of "nospam" and remove it
		idx := strings.Index(lower, "nospam")
		return email[:idx] + email[idx+6:]
	}
	return email
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

// validateURL checks for SSRF vulnerabilities.
func validateURL(urlStr string) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := strings.ToLower(parsed.Hostname())

	// Block localhost and local domains
	if host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return errors.New("blocked: local host")
	}

	// Block private IP ranges
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return errors.New("blocked: private IP")
		}
	}

	// Block metadata service endpoints
	if host == "169.254.169.254" || host == "metadata.google.internal" || host == "metadata.azure.com" {
		return errors.New("blocked: metadata service")
	}

	return nil
}
