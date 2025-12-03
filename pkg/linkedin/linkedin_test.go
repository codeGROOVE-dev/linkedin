package linkedin

import (
	"context"
	"log/slog"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.linkedin.com/in/johndoe", true},
		{"https://linkedin.com/in/johndoe", true},
		{"https://linkedin.com/in/johndoe/", true},
		{"linkedin.com/in/johndoe", true},
		{"https://LINKEDIN.COM/IN/johndoe", true},
		{"https://linkedin.com/company/acme", false},
		{"https://twitter.com/johndoe", false},
		{"https://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := Match(tt.url)
			if got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestAuthRequired(t *testing.T) {
	if !AuthRequired() {
		t.Error("LinkedIn should require auth")
	}
}

func TestExtractPublicID(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://linkedin.com/in/johndoe", "johndoe"},
		{"https://linkedin.com/in/johndoe/", "johndoe"},
		{"https://linkedin.com/in/john-doe-123", "john-doe-123"},
		{"https://example.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := extractPublicID(tt.url)
			if got != tt.want {
				t.Errorf("extractPublicID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	ctx := context.Background()

	t.Run("creates_client_without_error", func(t *testing.T) {
		// Auth is broken, so New() should succeed without cookies
		client, err := New(ctx)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		if client == nil {
			t.Fatal("New() returned nil client")
		}
	})

	t.Run("with_logger", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		client, err := New(ctx, WithLogger(logger))
		if err != nil {
			t.Fatalf("New(WithLogger) failed: %v", err)
		}
		if client == nil {
			t.Fatal("New(WithLogger) returned nil client")
		}
	})
}

func TestFetch(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	client, err := New(ctx, WithLogger(logger))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	t.Run("returns_minimal_profile", func(t *testing.T) {
		prof, err := client.Fetch(ctx, "https://www.linkedin.com/in/johndoe")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if prof == nil {
			t.Fatal("Fetch() returned nil profile")
		}
		if prof.Platform != "linkedin" {
			t.Errorf("Platform = %q, want %q", prof.Platform, "linkedin")
		}
		if prof.Username != "johndoe" {
			t.Errorf("Username = %q, want %q", prof.Username, "johndoe")
		}
		if prof.URL != "https://www.linkedin.com/in/johndoe" {
			t.Errorf("URL = %q, want %q", prof.URL, "https://www.linkedin.com/in/johndoe")
		}
		if prof.Authenticated {
			t.Error("Authenticated should be false (auth is broken)")
		}
	})

	t.Run("normalizes_url", func(t *testing.T) {
		prof, err := client.Fetch(ctx, "johndoe")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if prof.URL != "https://www.linkedin.com/in/johndoe" {
			t.Errorf("URL = %q, want normalized URL", prof.URL)
		}
	})
}
