package cache

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrCacheExpired_Is(t *testing.T) {
	err := ErrCacheExpired

	assert.True(t, errors.Is(err, ErrCacheExpired))
	assert.False(t, errors.Is(err, ErrCacheCorrupted))
	assert.False(t, errors.Is(err, ErrCacheFull))
	assert.False(t, errors.Is(err, ErrCacheInvalidated))
}

func TestErrCacheExpired_Error(t *testing.T) {
	err := ErrCacheExpired
	expected := "cache entry has expired"
	assert.Equal(t, expected, err.Error())
}

func TestErrCacheCorrupted_Is(t *testing.T) {
	err := ErrCacheCorrupted

	assert.True(t, errors.Is(err, ErrCacheCorrupted))
	assert.False(t, errors.Is(err, ErrCacheExpired))
	assert.False(t, errors.Is(err, ErrCacheFull))
	assert.False(t, errors.Is(err, ErrCacheInvalidated))
}

func TestErrCacheCorrupted_Error(t *testing.T) {
	err := ErrCacheCorrupted
	expected := "cache entry is corrupted"
	assert.Equal(t, expected, err.Error())
}

func TestErrCacheFull_Is(t *testing.T) {
	err := ErrCacheFull

	assert.True(t, errors.Is(err, ErrCacheFull))
	assert.False(t, errors.Is(err, ErrCacheExpired))
	assert.False(t, errors.Is(err, ErrCacheCorrupted))
	assert.False(t, errors.Is(err, ErrCacheInvalidated))
}

func TestErrCacheFull_Error(t *testing.T) {
	err := ErrCacheFull
	expected := "cache is full"
	assert.Equal(t, expected, err.Error())
}

func TestErrCacheInvalidated_Is(t *testing.T) {
	err := ErrCacheInvalidated

	assert.True(t, errors.Is(err, ErrCacheInvalidated))
	assert.False(t, errors.Is(err, ErrCacheExpired))
	assert.False(t, errors.Is(err, ErrCacheCorrupted))
	assert.False(t, errors.Is(err, ErrCacheFull))
}

func TestErrCacheInvalidated_Error(t *testing.T) {
	err := ErrCacheInvalidated
	expected := "cache entry has been invalidated"
	assert.Equal(t, expected, err.Error())
}

func TestErrorWrapping(t *testing.T) {
	tests := []struct {
		name       string
		baseError  error
		wrappedMsg string
	}{
		{
			name:       "wrap cache expired",
			baseError:  ErrCacheExpired,
			wrappedMsg: "failed to get manifest",
		},
		{
			name:       "wrap cache corrupted",
			baseError:  ErrCacheCorrupted,
			wrappedMsg: "failed to read entry",
		},
		{
			name:       "wrap cache full",
			baseError:  ErrCacheFull,
			wrappedMsg: "cannot store entry",
		},
		{
			name:       "wrap cache invalidated",
			baseError:  ErrCacheInvalidated,
			wrappedMsg: "entry no longer valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test direct wrapping with %w verb
			wrappedErr := fmt.Errorf("%s: %w", tt.wrappedMsg, tt.baseError)
			assert.Contains(t, wrappedErr.Error(), tt.baseError.Error())

			// Test that the original error is still detectable through wrapping
			assert.True(t, errors.Is(wrappedErr, tt.baseError))
		})
	}
}

func TestErrorUniqueness(t *testing.T) {
	// All errors should be distinct
	cacheErrors := []error{ErrCacheExpired, ErrCacheCorrupted, ErrCacheFull, ErrCacheInvalidated}

	for i, err1 := range cacheErrors {
		for j, err2 := range cacheErrors {
			if i != j {
				assert.False(
					t,
					errors.Is(err1, err2),
					"Error %v should not be equal to %v",
					err1,
					err2,
				)
			}
		}
	}
}
