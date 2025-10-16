package cache

import (
	"net/url"
	"path/filepath"
	"strings"
)

// normalizeURL normalizes a Git repository URL to a consistent filesystem-safe path.
//
// Normalization rules:
// 1. Strip .git suffix
// 2. Convert SSH URLs (git@host:path) to host/path format
// 3. Convert HTTPS/HTTP URLs (https://host/path) to host/path format
// 4. Remove trailing slashes
// 5. Convert to filesystem-safe path
//
// Examples:
//   - https://github.com/my/repo.git → github.com/my/repo
//   - git@github.com:my/repo → github.com/my/repo
//   - https://github.com/my/repo → github.com/my/repo
//   - http://gitlab.com/org/project → gitlab.com/org/project
func normalizeURL(rawURL string) string {
	// Remove .git suffix if present
	rawURL = strings.TrimSuffix(rawURL, ".git")

	// Handle SSH URLs (git@host:path)
	if strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") && !strings.Contains(rawURL, "://") {
		// Format: git@github.com:org/repo or user@host:path
		parts := strings.SplitN(rawURL, "@", 2)
		if len(parts) == 2 {
			// Split host and path
			hostPath := parts[1]
			hostPath = strings.Replace(hostPath, ":", "/", 1)
			return strings.TrimSuffix(hostPath, "/")
		}
	}

	// Handle HTTP(S) URLs
	parsed, err := url.Parse(rawURL)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		// Format: https://github.com/org/repo
		path := parsed.Host + parsed.Path
		return strings.TrimSuffix(path, "/")
	}

	// Fallback: clean up the URL as-is
	return strings.TrimSuffix(rawURL, "/")
}

// makeCompositeKey creates a composite key from URL, ref, and cache key.
// Format: normalizedURL/ref/cacheKey
//
// Example:
//   - (https://github.com/my/repo, main, team-docs) → github.com/my/repo/main/team-docs
func makeCompositeKey(rawURL, ref, cacheKey string) string {
	normalized := normalizeURL(rawURL)
	return filepath.Join(normalized, ref, cacheKey)
}
