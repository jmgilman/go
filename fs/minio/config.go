// Package minio provides a MinIO/S3-compatible implementation of the core.FS interface.
package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7"
)

// Config holds MinIO filesystem configuration.
type Config struct {
	// Endpoint is the MinIO server URL (e.g., "localhost:9000")
	Endpoint string

	// Bucket is the S3 bucket name
	Bucket string

	// AccessKey is the access key ID for authentication
	AccessKey string

	// SecretKey is the secret access key for authentication
	SecretKey string

	// UseSSL enables HTTPS connections (default: true)
	UseSSL bool

	// Prefix is an optional prefix for all object keys (for namespacing)
	Prefix string

	// Client is an optional pre-configured MinIO client
	// If provided, Endpoint/AccessKey/SecretKey are ignored
	Client *minio.Client

	// MultipartThreshold is the file size threshold for multipart uploads
	// Default: 5MB (MinIO SDK default)
	// Set to 0 to use SDK default
	MultipartThreshold int64

	// MaxRenameConcurrency limits concurrent copies during directory rename
	// Default: 10
	// Higher values speed up renames but increase server load
	MaxRenameConcurrency int
}

// validate checks if the configuration is valid.
// Either Client OR (Endpoint + Bucket + AccessKey + SecretKey) must be provided.
func (c *Config) validate() error {
	// Check if bucket is provided (required in all cases)
	if c.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}

	// If Client is provided, we're done (other fields are ignored)
	if c.Client != nil {
		return nil
	}

	// Otherwise, check required connection fields
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required when client is not provided")
	}
	if c.AccessKey == "" {
		return fmt.Errorf("access key is required when client is not provided")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("secret key is required when client is not provided")
	}

	return nil
}
