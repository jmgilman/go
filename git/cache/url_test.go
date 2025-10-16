package cache

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTTPS with .git suffix",
			input:    "https://github.com/my/repo.git",
			expected: "github.com/my/repo",
		},
		{
			name:     "HTTPS without .git suffix",
			input:    "https://github.com/my/repo",
			expected: "github.com/my/repo",
		},
		{
			name:     "SSH format with .git suffix",
			input:    "git@github.com:my/repo.git",
			expected: "github.com/my/repo",
		},
		{
			name:     "SSH format without .git suffix",
			input:    "git@github.com:my/repo",
			expected: "github.com/my/repo",
		},
		{
			name:     "HTTP with .git suffix",
			input:    "http://gitlab.com/org/project.git",
			expected: "gitlab.com/org/project",
		},
		{
			name:     "HTTP without .git suffix",
			input:    "http://gitlab.com/org/project",
			expected: "gitlab.com/org/project",
		},
		{
			name:     "SSH with different user",
			input:    "user@example.com:org/repo.git",
			expected: "example.com/org/repo",
		},
		{
			name:     "URL with trailing slash",
			input:    "https://github.com/my/repo/",
			expected: "github.com/my/repo",
		},
		{
			name:     "Nested path",
			input:    "https://gitlab.com/group/subgroup/repo.git",
			expected: "gitlab.com/group/subgroup/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMakeCompositeKey(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		ref      string
		cacheKey string
		expected string
	}{
		{
			name:     "HTTPS URL with branch and stable key",
			url:      "https://github.com/my/repo.git",
			ref:      "main",
			cacheKey: "team-docs",
			expected: "github.com/my/repo/main/team-docs",
		},
		{
			name:     "SSH URL with tag and UUID",
			url:      "git@github.com:my/repo",
			ref:      "v1.0.0",
			cacheKey: "build-abc123",
			expected: "github.com/my/repo/v1.0.0/build-abc123",
		},
		{
			name:     "Different refs create different keys",
			url:      "https://github.com/my/repo",
			ref:      "develop",
			cacheKey: "dev-checkout",
			expected: "github.com/my/repo/develop/dev-checkout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeCompositeKey(tt.url, tt.ref, tt.cacheKey)
			if result != tt.expected {
				t.Errorf("makeCompositeKey(%q, %q, %q) = %q, want %q",
					tt.url, tt.ref, tt.cacheKey, result, tt.expected)
			}
		})
	}
}
