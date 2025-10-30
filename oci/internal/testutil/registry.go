// Package testutil provides testing utilities for the OCI bundle library.
// It includes test registries, archive generators, and benchmark utilities.
package testutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestRegistry provides a Docker registry running in a test container.
// It automatically handles startup, shutdown, and cleanup.
type TestRegistry struct {
	container testcontainers.Container
	host      string
	port      int
	reference string
}

// NewTestRegistry creates and starts a new test registry container.
// It returns a TestRegistry that can be used to get the registry reference.
//
// The registry is configured with:
// - Anonymous access (no authentication required)
// - Storage driver: filesystem (in-memory for testing)
//
// Example usage:
//
//	registry, err := NewTestRegistry(ctx)
//	if err != nil {
//	    t.Fatal(err)
//	}
//	defer registry.Close(ctx)
//
//	ref := registry.Reference()
//	// Use ref like "localhost:5000/test-repo:latest"
func NewTestRegistry(ctx context.Context) (*TestRegistry, error) {
	// Allow overriding the registry image for compatibility testing
	image := os.Getenv("TEST_REGISTRY_IMAGE")
	if image == "" {
		image = "ghcr.io/project-zot/zot:latest"
	}

	req := testcontainers.ContainerRequest{
		Image:        image,
		ExposedPorts: []string{"5000/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("5000/tcp"),
			wait.ForHTTP("/v2/").
				WithPort("5000/tcp").
				WithAllowInsecure(true).
				WithStatusCodeMatcher(func(code int) bool {
					// Many registries return 200, 401, or 403 for /v2/
					return code == http.StatusOK || code == http.StatusUnauthorized || code == http.StatusForbidden
				}),
		),
		// Keep Docker Distribution env; harmless for other images
		Env: map[string]string{
			"REGISTRY_HTTP_ADDR":            "0.0.0.0:5000",
			"REGISTRY_HTTP_TLS_CERTIFICATE": "",
			"REGISTRY_HTTP_TLS_KEY":         "",
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start registry container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		if terminateErr := container.Terminate(ctx); terminateErr != nil {
			// Log cleanup error but don't fail the operation
			fmt.Printf("Warning: failed to terminate container during cleanup: %v\n", terminateErr)
		}
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5000")
	if err != nil {
		if terminateErr := container.Terminate(ctx); terminateErr != nil {
			// Log cleanup error but don't fail the operation
			fmt.Printf("Warning: failed to terminate container during cleanup: %v\n", terminateErr)
		}
		return nil, fmt.Errorf("failed to get container port: %w", err)
	}

	return &TestRegistry{
		container: container,
		host:      host,
		port:      port.Int(),
		reference: fmt.Sprintf("%s:%d", host, port.Int()),
	}, nil
}

// Reference returns the full registry reference that can be used with the Client.
// The reference includes the host and port, e.g., "localhost:5000".
func (r *TestRegistry) Reference() string {
	return r.reference
}

// URL returns the full registry URL including protocol.
func (r *TestRegistry) URL() string {
	return fmt.Sprintf("http://%s", r.reference)
}

// Close terminates the registry container and cleans up resources.
// It should be called in test cleanup (defer statement).
func (r *TestRegistry) Close(ctx context.Context) error {
	if r.container != nil {
		if err := r.container.Terminate(ctx); err != nil {
			return fmt.Errorf("failed to terminate registry container: %w", err)
		}
	}
	return nil
}

// WaitForReady waits for the registry to be ready to accept connections.
// It performs a health check by attempting to connect to the registry port.
func (r *TestRegistry) WaitForReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for registry to be ready: %w", ctx.Err())
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", r.reference, 1*time.Second)
			if err == nil {
				conn.Close()
				return nil // registry is ready
			}
		}
	}
}
