// Package oras provides ORAS wrapper functionality.
// This file contains interface definitions for testing and dependency injection.
package oras

import "context"

//go:generate go run github.com/matryer/moq@v0.5.3 -pkg mocks -out mocks/oras_client.go . Client

// Client defines the interface for ORAS operations that can be mocked for testing.
type Client interface {
	// Push pushes an artifact to an OCI registry.
	Push(ctx context.Context, reference string, descriptor *PushDescriptor, opts *AuthOptions) error

	// Pull pulls an artifact from an OCI registry.
	Pull(ctx context.Context, reference string, opts *AuthOptions) (*PullDescriptor, error)
}
