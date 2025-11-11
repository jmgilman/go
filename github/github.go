// Package github provides a clean, idiomatic wrapper around GitHub operations.
package github

import (
	"time"

	"github.com/jmgilman/go/errors"
)

// ParseGitHubTime parses a timestamp string from the GitHub API.
// GitHub uses RFC3339 format for timestamps.
func ParseGitHubTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errors.Wrap(err, errors.CodeInvalidInput, "failed to parse timestamp")
	}
	return t, nil
}
