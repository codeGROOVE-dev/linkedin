package generic

import "testing"

func TestMatch(t *testing.T) {
	// Generic always matches
	tests := []string{
		"https://example.com",
		"https://random-site.org/profile",
		"anything",
	}

	for _, url := range tests {
		if !Match(url) {
			t.Errorf("Match(%q) should always return true", url)
		}
	}
}

func TestAuthRequired(t *testing.T) {
	if AuthRequired() {
		t.Error("Generic should not require auth")
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com", false},
		{"https://localhost", true},
		{"https://127.0.0.1", true},
		{"https://192.168.1.1", true},
		{"https://10.0.0.1", true},
		{"https://169.254.169.254", true},
		{"https://metadata.google.internal", true},
		{"https://foo.local", true},
		{"https://foo.internal", true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestCleanEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"website@nospamtpope.org", "website@tpope.org"},
		{"contact@NOSPAMexample.com", "contact@example.com"},
		{"user@NoSpAmtest.org", "user@test.org"},
		{"normal@example.com", "normal@example.com"},
		{"test@nospam.nospam.org", "test@.nospam.org"}, // Only removes first occurrence
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanEmail(tt.input)
			if got != tt.want {
				t.Errorf("cleanEmail(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
