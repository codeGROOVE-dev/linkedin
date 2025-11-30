package vkontakte

import (
	"context"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://vk.com/johndoe", true},
		{"https://vk.com/id12345", true},
		{"https://VK.COM/johndoe", true},
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
	if AuthRequired() {
		t.Error("VKontakte should not strictly require auth (cookies optional for bot detection)")
	}
}

func TestNewWithoutCookies(t *testing.T) {
	client, err := New(context.Background())
	if err != nil {
		t.Errorf("New() without cookies should succeed (cookies optional): %v", err)
	}
	if client == nil {
		t.Error("client should not be nil")
	}
}

func TestExtractUsername(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://vk.com/xrock", "xrock"},
		{"https://vk.com/id12345", "id12345"},
		{"vk.com/johndoe", "johndoe"},
		{"https://www.vk.com/username", "username"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := extractUsername(tt.url)
			if got != tt.want {
				t.Errorf("extractUsername(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
