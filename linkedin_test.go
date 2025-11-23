package linkedin

import (
	"testing"
)

func TestExtractField(t *testing.T) {
	tests := []struct {
		name  string
		html  string
		start string
		end   string
		want  string
	}{
		{
			name:  "basic extraction",
			html:  `{"firstName":"Thomas","lastName":"Stromberg"}`,
			start: `"firstName":"`,
			end:   `"`,
			want:  "Thomas",
		},
		{
			name:  "not found",
			html:  `{"firstName":"Thomas"}`,
			start: `"middleName":"`,
			end:   `"`,
			want:  "",
		},
		{
			name:  "empty value",
			html:  `{"firstName":""}`,
			start: `"firstName":"`,
			end:   `"`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractField(tt.html, tt.start, tt.end)
			if got != tt.want {
				t.Errorf("extractField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnescapeJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unicode escape",
			in:   `Thomas Str\u00f6mberg`,
			want: "Thomas Strömberg",
		},
		{
			name: "no escape",
			in:   "Thomas Stromberg",
			want: "Thomas Stromberg",
		},
		{
			name: "quote escape",
			in:   `Hello \"World\"`,
			want: `Hello "World"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeJSON(tt.in)
			if got != tt.want {
				t.Errorf("unescapeJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractMetaContent(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		property string
		want     string
	}{
		{
			name:     "og:title",
			html:     `<meta property="og:title" content="Thomas Stromberg" />`,
			property: `property="og:title"`,
			want:     "Thomas Stromberg",
		},
		{
			name:     "og:description",
			html:     `<meta property="og:description" content="Software Engineer at Google" />`,
			property: `property="og:description"`,
			want:     "Software Engineer at Google",
		},
		{
			name:     "not found",
			html:     `<meta property="og:title" content="Test" />`,
			property: `property="og:image"`,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMetaContent(tt.html, tt.property)
			if got != tt.want {
				t.Errorf("extractMetaContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCompanyFromHeadline(t *testing.T) {
	tests := []struct {
		name     string
		headline string
		want     string
	}{
		{
			name:     "Google Chrome product",
			headline: "Staff Software Engineer, Google Chrome",
			want:     "Google",
		},
		{
			name:     "at pattern",
			headline: "Software Engineer at Microsoft",
			want:     "Microsoft",
		},
		{
			name:     "at pattern with product",
			headline: "Senior Engineer at Amazon Web Services",
			want:     "Amazon",
		},
		{
			name:     "@ pattern",
			headline: "Product Manager @ Facebook Reality Labs",
			want:     "Facebook",
		},
		{
			name:     "two-word company name",
			headline: "Software Engineer, Red Hat",
			want:     "Red Hat",
		},
		{
			name:     "Goldman Sachs",
			headline: "Analyst at Goldman Sachs",
			want:     "Goldman Sachs",
		},
		{
			name:     "Wells Fargo",
			headline: "Vice President, Wells Fargo",
			want:     "Wells Fargo",
		},
		{
			name:     "single word company",
			headline: "Engineer, Apple",
			want:     "Apple",
		},
		{
			name:     "with trailing comma",
			headline: "Director at Intel, USA",
			want:     "Intel",
		},
		{
			name:     "no pattern match",
			headline: "Software Engineer",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCompanyFromHeadline(tt.headline)
			if got != tt.want {
				t.Errorf("parseCompanyFromHeadline(%q) = %q, want %q", tt.headline, got, tt.want)
			}
		})
	}
}

func TestExtractGeoLocation(t *testing.T) {
	tests := []struct {
		name           string
		fullBlock      string
		profileSection string
		want           string
	}{
		{
			name: "San Francisco Bay Area",
			fullBlock: `{
				"included":[
					{"entityUrn":"urn:li:fsd_geo:90000084","defaultLocalizedName":"San Francisco Bay Area","$type":"com.linkedin.voyager.dash.common.Geo"},
					{"entityUrn":"urn:li:fsd_geo:103644278","defaultLocalizedName":"United States","$type":"com.linkedin.voyager.dash.common.Geo"}
				]
			}`,
			profileSection: `{"*geo":"urn:li:fsd_geo:90000084","$type":"com.linkedin.voyager.dash.identity.profile.ProfileGeoLocation"}`,
			want:           "San Francisco Bay Area",
		},
		{
			name: "no geo field",
			fullBlock: `{
				"included":[
					{"entityUrn":"urn:li:fsd_geo:90000084","defaultLocalizedName":"San Francisco Bay Area"}
				]
			}`,
			profileSection: `{"firstName":"John","lastName":"Doe"}`,
			want:           "",
		},
		{
			name: "geo URN not found",
			fullBlock: `{
				"included":[
					{"entityUrn":"urn:li:fsd_geo:12345","defaultLocalizedName":"New York City"}
				]
			}`,
			profileSection: `{"*geo":"urn:li:fsd_geo:99999"}`,
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGeoLocation(tt.fullBlock, tt.profileSection)
			if got != tt.want {
				t.Errorf("extractGeoLocation() = %q, want %q", got, tt.want)
			}
		})
	}
}
