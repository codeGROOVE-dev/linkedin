package mastodon

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://mastodon.social/@johndoe", true},
		{"https://fosstodon.org/@johndoe", true},
		{"https://hachyderm.io/@johndoe", true},
		{"https://infosec.exchange/@johndoe", true},
		{"https://example.social/@johndoe", true},
		{"https://mastodon.social/users/johndoe", true},
		{"https://twitter.com/johndoe", false},
		{"https://linkedin.com/in/johndoe", false},
		{"https://example.com/about", false},
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
		t.Error("Mastodon should not require auth")
	}
}

func TestExtractUsername(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/@johndoe", "johndoe"},
		{"/users/johndoe", "johndoe"},
		{"/@johndoe/followers", "johndoe"},
		{"/about", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractUsername(tt.path)
			if got != tt.want {
				t.Errorf("extractUsername(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic paragraph",
			input: "<p>Hello</p>",
			want:  "Hello",
		},
		{
			name:  "multiple paragraphs",
			input: "<p>Hello</p><p>World</p>",
			want:  "Hello\nWorld",
		},
		{
			name:  "HTML entities",
			input: "Hello &amp; World",
			want:  "Hello & World",
		},
		{
			name:  "links",
			input: "<a href='url'>link</a>",
			want:  "link",
		},
		{
			name:  "br tag",
			input: "Line 1<br>Line 2",
			want:  "Line 1\nLine 2",
		},
		{
			name:  "br self-closing",
			input: "Line 1<br/>Line 2",
			want:  "Line 1\nLine 2",
		},
		{
			name:  "br with space",
			input: "Line 1<br />Line 2",
			want:  "Line 1\nLine 2",
		},
		{
			name:  "div tags",
			input: "<div>Block 1</div><div>Block 2</div>",
			want:  "Block 1\nBlock 2",
		},
		{
			name:  "complex bio with multiple breaks",
			input: "KD4UHP - based out of Carrboro, NC<br>founder &amp; CEO @ codeGROOVE<br />former Director of Security @ Chainguard &amp; Xoogler<br/>#unix #infosec #bikes",
			want:  "KD4UHP - based out of Carrboro, NC\nfounder & CEO @ codeGROOVE\nformer Director of Security @ Chainguard & Xoogler\n#unix #infosec #bikes",
		},
		{
			name:  "empty lines removed",
			input: "<p>Line 1</p><p></p><p>Line 2</p>",
			want:  "Line 1\nLine 2",
		},
		{
			name:  "whitespace normalized",
			input: "<p>  Line 1  </p><br/><p>   Line 2   </p>",
			want:  "Line 1\nLine 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.input)
			if got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
