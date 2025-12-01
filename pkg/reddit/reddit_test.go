package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"user path", "https://reddit.com/user/username", true},
		{"u path", "https://reddit.com/u/username", true},
		{"old reddit", "https://old.reddit.com/user/username", true},
		{"www reddit", "https://www.reddit.com/user/username", true},
		{"subreddit", "https://reddit.com/r/golang", false},
		{"other domain", "https://twitter.com/user", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestAuthRequired(t *testing.T) {
	if AuthRequired() {
		t.Error("AuthRequired() = true, want false")
	}
}

func TestExtractUsername(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"user path", "https://reddit.com/user/johndoe", "johndoe"},
		{"u path", "https://reddit.com/u/johndoe", "johndoe"},
		{"with trailing slash", "https://reddit.com/user/johndoe/", "johndoe"},
		{"old reddit", "https://old.reddit.com/user/username", "username"},
		{"invalid", "https://reddit.com/r/golang", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractUsername(tt.url); got != tt.want {
				t.Errorf("extractUsername(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	client, err := New(ctx)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if client == nil {
		t.Fatal("New() returned nil client")
	}
}

func TestFetch(t *testing.T) {
	// Create a mock server that returns Reddit-like HTML
	mockHTML := `<!DOCTYPE html>
<html>
<head><title>overview for testuser - Reddit</title></head>
<body>
<div class="side">
  <span class="karma">1,234 post karma</span>
  <span class="karma">5,678 comment karma</span>
  <span>redditor since 2020</span>
</div>
<div class="thing" data-subreddit="golang">
  <div class="md"><p>This is a sample comment about Go programming and testing.</p></div>
</div>
<div class="thing" data-subreddit="programming">
  <div class="md"><p>Another comment about software development practices.</p></div>
</div>
<a href="https://github.com/testuser">GitHub</a>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := New(ctx)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Override the httpClient to use our mock server
	client.httpClient = server.Client()

	// We need to intercept the request to redirect to our mock server
	// Create a custom transport that redirects old.reddit.com to our test server
	originalTransport := client.httpClient.Transport
	client.httpClient.Transport = &mockTransport{
		mockURL:   server.URL,
		transport: originalTransport,
	}

	profile, err := client.Fetch(ctx, "https://reddit.com/user/testuser")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if profile.Platform != "reddit" {
		t.Errorf("Platform = %q, want %q", profile.Platform, "reddit")
	}
	if profile.Username != "testuser" {
		t.Errorf("Username = %q, want %q", profile.Username, "testuser")
	}
	if profile.Name != "testuser" {
		t.Errorf("Name = %q, want %q", profile.Name, "testuser")
	}
}

// mockTransport redirects requests to the mock server
type mockTransport struct {
	mockURL   string
	transport http.RoundTripper
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect to mock server
	req.URL.Scheme = "http"
	req.URL.Host = t.mockURL[7:] // Strip "http://"
	if t.transport != nil {
		return t.transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestFetch_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := New(ctx)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client.httpClient.Transport = &mockTransport{mockURL: server.URL}

	_, err = client.Fetch(ctx, "https://reddit.com/user/nonexistent")
	if err == nil {
		t.Error("Fetch() expected error for 404, got nil")
	}
}

func TestParseProfile(t *testing.T) {
	tests := []struct {
		name           string
		html           string
		wantUsername   string
		wantName       string
		wantPostKarma  string
		wantSubreddits string
	}{
		{
			name: "full profile",
			html: `<html><head><title>overview for johndoe - Reddit</title></head><body>
				<span>1,234 post karma</span>
				<span>5,678 comment karma</span>
				<span>redditor since 2019</span>
				<div data-subreddit="golang"></div>
				<div data-subreddit="rust"></div>
			</body></html>`,
			wantUsername:   "johndoe",
			wantName:       "johndoe",
			wantPostKarma:  "1234",
			wantSubreddits: "golang, rust",
		},
		{
			name:         "minimal profile",
			html:         `<html><head><title>overview for minuser - Reddit</title></head><body></body></html>`,
			wantUsername: "minuser",
			wantName:     "minuser",
		},
		{
			name:         "empty title fallback",
			html:         `<html><head><title></title></head><body></body></html>`,
			wantUsername: "fallback",
			wantName:     "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := parseProfile(tt.html, "https://old.reddit.com/user/"+tt.wantUsername, tt.wantUsername)
			if err != nil {
				t.Fatalf("parseProfile() error = %v", err)
			}

			if profile.Username != tt.wantUsername {
				t.Errorf("Username = %q, want %q", profile.Username, tt.wantUsername)
			}
			if profile.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", profile.Name, tt.wantName)
			}
			if tt.wantPostKarma != "" && profile.Fields["post_karma"] != tt.wantPostKarma {
				t.Errorf("post_karma = %q, want %q", profile.Fields["post_karma"], tt.wantPostKarma)
			}
			if tt.wantSubreddits != "" && profile.Fields["subreddits"] != tt.wantSubreddits {
				t.Errorf("subreddits = %q, want %q", profile.Fields["subreddits"], tt.wantSubreddits)
			}
		})
	}
}

func TestExtractSubreddits(t *testing.T) {
	html := `<div data-subreddit="golang"></div>
		<div data-subreddit="rust"></div>
		<div data-subreddit="u_someuser"></div>
		<div data-subreddit="AskReddit"></div>
		<div data-subreddit="kubernetes"></div>`

	subs := extractSubreddits(html)

	// Should include golang, rust, kubernetes but not u_someuser (user profile) or AskReddit (generic)
	if len(subs) != 3 {
		t.Errorf("extractSubreddits() returned %d subreddits, want 3: %v", len(subs), subs)
	}

	expected := map[string]bool{"golang": true, "rust": true, "kubernetes": true}
	for _, sub := range subs {
		if !expected[sub] {
			t.Errorf("unexpected subreddit: %q", sub)
		}
	}
}

func TestExtractCommentSamples(t *testing.T) {
	html := `<div class="md"><p>This is a longer comment that should be included in the samples.</p></div>
		<div class="md"><p>Short</p></div>
		<div class="md"><p>Another good comment that has enough content to be included.</p></div>
		<div class="md"><p>This post is archived automatically archived.</p></div>`

	samples := extractCommentSamples(html, 5)

	// Should include the two longer comments but not the short one or archived one
	if len(samples) != 2 {
		t.Errorf("extractCommentSamples() returned %d samples, want 2: %v", len(samples), samples)
	}
}

func TestIsGenericSubreddit(t *testing.T) {
	tests := []struct {
		sub  string
		want bool
	}{
		{"AskReddit", true},
		{"pics", true},
		{"golang", false},
		{"kubernetes", false},
		{"rust", false},
	}

	for _, tt := range tests {
		t.Run(tt.sub, func(t *testing.T) {
			if got := isGenericSubreddit(tt.sub); got != tt.want {
				t.Errorf("isGenericSubreddit(%q) = %v, want %v", tt.sub, got, tt.want)
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<p>Hello</p>", "Hello"},
		{"&lt;script&gt;", "<script>"},
		{"&amp;&quot;&#39;", "&\"'"},
		{"Hello&nbsp;World", "Hello World"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripHTML(tt.input); got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
