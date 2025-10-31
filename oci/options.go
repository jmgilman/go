// Package ocibundle provides OCI bundle distribution functionality.
// This file contains functional options for configuration.
package ocibundle

import (
	"context"
	"time"

	"github.com/jmgilman/go/fs/core"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/jmgilman/go/oci/internal/cache"
	"github.com/jmgilman/go/oci/internal/oras"
)

// ClientOptions contains configuration options for the Client.
type ClientOptions struct {
	// Auth options for ORAS operations
	Auth *oras.AuthOptions

	// ORASClient allows injecting a custom ORAS client for testing
	// If nil, the default ORAS client will be used
	ORASClient oras.Client

	// HTTPConfig controls HTTP vs HTTPS and certificate validation
	HTTPConfig *HTTPConfig

	// FS provides filesystem operations for archive/temp/extraction handling.
	// If nil, a default OS-backed filesystem will be used.
	FS core.FS

	// CacheConfig contains cache configuration for OCI operations.
	// If nil, caching is disabled.
	CacheConfig *CacheConfig
}

// HTTPConfig contains configuration for HTTP transport settings.
// This allows explicit control over HTTP usage and certificate validation,
// rather than relying on brittle localhost detection.
type HTTPConfig struct {
	// AllowHTTP enables HTTP instead of HTTPS for registry connections.
	// This is useful for local registries that don't support HTTPS.
	AllowHTTP bool

	// AllowInsecure allows connections to registries with self-signed
	// or invalid certificates. This should only be used for testing.
	AllowInsecure bool

	// Registries specifies which registries this configuration applies to.
	// If empty, applies to all registries. Supports hostname matching.
	Registries []string
}

// CacheConfig contains configuration for OCI caching behavior.
type CacheConfig struct {
	// Coordinator provides the cache implementation.
	// If nil, caching is disabled.
	Coordinator cache.Cache

	// CachePath specifies the filesystem path for cache storage.
	// If empty and Coordinator is nil, no cache directory is created.
	CachePath string

	// Policy controls when caching should be used.
	Policy CachePolicy

	// MaxSizeBytes is the maximum size of the cache in bytes.
	// Defaults to 1GB if not specified.
	MaxSizeBytes int64

	// DefaultTTL is the default time-to-live for cache entries.
	// Defaults to 24 hours if not specified.
	DefaultTTL time.Duration
}

// CachePolicy defines when caching should be applied to operations.
type CachePolicy string

const (
	// CachePolicyDisabled disables caching completely.
	CachePolicyDisabled CachePolicy = "disabled"

	// CachePolicyEnabled enables caching for all operations.
	CachePolicyEnabled CachePolicy = "enabled"

	// CachePolicyPull enables caching only for pull operations.
	CachePolicyPull CachePolicy = "pull"

	// CachePolicyPush enables caching only for push operations.
	CachePolicyPush CachePolicy = "push"
)

// ClientOption is a functional option for configuring the Client.
type ClientOption func(*ClientOptions)

// WithAuthNone configures the client to rely on ORAS's default Docker credential chain.
// This is the default behavior and uses ~/.docker/config.json and credential helpers
// like osxkeychain, pass, desktop, etc. as configured by the user.
func WithAuthNone() ClientOption {
	return func(opts *ClientOptions) {
		// Explicitly set to nil to ensure default behavior
		opts.Auth = nil
	}
}

// WithORASClient configures the client to use a custom ORAS client.
// This is primarily used for testing to inject mock implementations.
func WithORASClient(client oras.Client) ClientOption {
	return func(opts *ClientOptions) {
		opts.ORASClient = client
	}
}

// WithStaticAuth configures static credentials for a specific registry.
// This overrides the default Docker credential chain for the specified registry
// but allows other registries to use the default chain.
//
// Parameters:
//   - registry: The registry hostname (e.g., "ghcr.io")
//   - username: Username for authentication
//   - password: Password for authentication
//
// For other registries not matching the specified one, the default Docker
// credential chain will be used.
func WithStaticAuth(registry, username, password string) ClientOption {
	return func(opts *ClientOptions) {
		if opts.Auth == nil {
			opts.Auth = &oras.AuthOptions{}
		}
		opts.Auth.StaticRegistry = registry
		opts.Auth.StaticUsername = username
		opts.Auth.StaticPassword = password
	}
}

// WithCredentialFunc configures a custom credential callback function.
// This completely overrides the default Docker credential chain and provides
// full control over credential resolution for all registries.
//
// Parameters:
//   - fn: Function that returns credentials for a given registry.
//     Return empty credentials to fall back to anonymous access.
//     Return an error to fail authentication for that registry.
//
// The function should be safe for concurrent use and handle context cancellation.
func WithCredentialFunc(fn func(ctx context.Context, registry string) (auth.Credential, error)) ClientOption {
	return func(opts *ClientOptions) {
		if opts.Auth == nil {
			opts.Auth = &oras.AuthOptions{}
		}
		opts.Auth.CredentialFunc = fn
	}
}

// WithHTTP configures HTTP transport settings for registry connections.
// This allows explicit control over HTTP vs HTTPS usage and certificate validation.
//
// Parameters:
//   - allowHTTP: Enable HTTP instead of HTTPS for registry connections
//   - allowInsecure: Allow connections to registries with self-signed certificates
//   - registries: Specific registries to apply this config to (empty applies to all)
//
// Example usage:
//
//	client, err := New(WithHTTP(true, true, []string{"localhost:5000"}))
//
// This is preferred over automatic localhost detection for better control and testability.
func WithHTTP(allowHTTP, allowInsecure bool, registries []string) ClientOption {
	return func(opts *ClientOptions) {
		opts.HTTPConfig = &HTTPConfig{
			AllowHTTP:     allowHTTP,
			AllowInsecure: allowInsecure,
			Registries:    registries,
		}
	}
}

// WithAllowHTTP is a convenience function for enabling HTTP connections.
// This enables HTTP for all registries, useful for local development.
//
// Example usage:
//
//	client, err := New(WithAllowHTTP())
func WithAllowHTTP() ClientOption {
	return WithHTTP(true, false, nil)
}

// WithInsecureHTTP is a convenience function for enabling insecure HTTP connections.
// This enables both HTTP and allows self-signed certificates for all registries.
// WARNING: Only use this for testing environments.
//
// Example usage:
//
//	client, err := New(WithInsecureHTTP())
func WithInsecureHTTP() ClientOption {
	return WithHTTP(true, true, nil)
}

// PushOptions contains options for the Push operation.
type PushOptions struct {
	// Annotations to attach to the OCI artifact manifest
	Annotations map[string]string

	// Platform specifies the target platform for the artifact
	Platform string

	// ProgressCallback is called during push operations to report progress
	ProgressCallback func(current, total int64)

	// MaxRetries is the maximum number of retry attempts for network operations
	MaxRetries int

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration

	// CacheBypass disables caching for this specific push operation.
	// When true, the operation will bypass any configured cache.
	CacheBypass bool
}

// PushOption is a functional option for configuring Push operations.
type PushOption func(*PushOptions)

// WithAnnotations sets annotations to be attached to the OCI artifact.
func WithAnnotations(annotations map[string]string) PushOption {
	return func(opts *PushOptions) {
		if opts.Annotations == nil {
			opts.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			opts.Annotations[k] = v
		}
	}
}

// WithPlatform sets the target platform for the OCI artifact.
func WithPlatform(platform string) PushOption {
	return func(opts *PushOptions) {
		opts.Platform = platform
	}
}

// WithProgressCallback sets a callback function for progress reporting.
func WithProgressCallback(callback func(current, total int64)) PushOption {
	return func(opts *PushOptions) {
		opts.ProgressCallback = callback
	}
}

// WithMaxRetries sets the maximum number of retry attempts for network operations.
func WithMaxRetries(maxRetries int) PushOption {
	return func(opts *PushOptions) {
		opts.MaxRetries = maxRetries
	}
}

// WithRetryDelay sets the delay between retry attempts.
func WithRetryDelay(delay time.Duration) PushOption {
	return func(opts *PushOptions) {
		opts.RetryDelay = delay
	}
}

// WithPushCacheBypass disables caching for this push operation.
func WithPushCacheBypass(bypass bool) PushOption {
	return func(opts *PushOptions) {
		opts.CacheBypass = bypass
	}
}

// PullOptions contains options for the Pull operation.
type PullOptions struct {
	// MaxFiles is the maximum number of files allowed in the archive.
	// Set to 0 for unlimited (not recommended for security).
	MaxFiles int

	// MaxSize is the maximum total uncompressed size of all files combined.
	// Set to 0 for unlimited (not recommended for security).
	MaxSize int64

	// MaxFileSize is the maximum size allowed for any individual file.
	// Set to 0 for unlimited (not recommended for security).
	MaxFileSize int64

	// AllowHiddenFiles determines whether hidden files (starting with .) are allowed.
	AllowHiddenFiles bool

	// PreservePermissions determines whether to preserve original file permissions.
	// When false, permissions are sanitized for security.
	PreservePermissions bool

	// StripPrefix removes this prefix from all file paths during extraction.
	// Useful for removing leading directory names from archived paths.
	StripPrefix string

	// MaxRetries is the maximum number of retry attempts for network operations.
	MaxRetries int

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration

	// CacheBypass disables caching for this specific pull operation.
	// When true, the operation will bypass any configured cache.
	CacheBypass bool

	// FilesToExtract specifies glob patterns for selective file extraction.
	// When non-empty, only files matching at least one pattern will be extracted.
	// Supports standard glob patterns:
	//   - *.json: matches all .json files in root
	//   - config/*: matches all files in config directory
	//   - **/*.txt: matches all .txt files recursively
	// When empty, all files are extracted (default behavior).
	FilesToExtract []string
}

// PullOption is a functional option for configuring Pull operations.
type PullOption func(*PullOptions)

// WithPullMaxFiles sets the maximum number of files allowed in the archive.
func WithPullMaxFiles(maxFiles int) PullOption {
	return func(opts *PullOptions) {
		opts.MaxFiles = maxFiles
	}
}

// WithPullMaxSize sets the maximum total uncompressed size of all files combined.
func WithPullMaxSize(maxSize int64) PullOption {
	return func(opts *PullOptions) {
		opts.MaxSize = maxSize
	}
}

// WithPullMaxFileSize sets the maximum size allowed for any individual file.
func WithPullMaxFileSize(maxFileSize int64) PullOption {
	return func(opts *PullOptions) {
		opts.MaxFileSize = maxFileSize
	}
}

// WithPullAllowHiddenFiles determines whether hidden files are allowed.
func WithPullAllowHiddenFiles(allow bool) PullOption {
	return func(opts *PullOptions) {
		opts.AllowHiddenFiles = allow
	}
}

// WithPullPreservePermissions determines whether to preserve original file permissions.
func WithPullPreservePermissions(preserve bool) PullOption {
	return func(opts *PullOptions) {
		opts.PreservePermissions = preserve
	}
}

// WithPullStripPrefix sets the prefix to remove from all file paths during extraction.
func WithPullStripPrefix(prefix string) PullOption {
	return func(opts *PullOptions) {
		opts.StripPrefix = prefix
	}
}

// WithPullMaxRetries sets the maximum number of retry attempts for network operations.
func WithPullMaxRetries(maxRetries int) PullOption {
	return func(opts *PullOptions) {
		opts.MaxRetries = maxRetries
	}
}

// WithPullRetryDelay sets the delay between retry attempts.
func WithPullRetryDelay(delay time.Duration) PullOption {
	return func(opts *PullOptions) {
		opts.RetryDelay = delay
	}
}

// WithPullCacheBypass disables caching for this pull operation.
func WithPullCacheBypass(bypass bool) PullOption {
	return func(opts *PullOptions) {
		opts.CacheBypass = bypass
	}
}

// WithFilesToExtract specifies glob patterns for selective file extraction.
// Only files matching at least one pattern will be extracted from the archive.
// This enables bandwidth savings when used with eStargz archives and HTTP Range requests.
//
// Supported glob patterns:
//   - "*.json" - matches all .json files in the root directory
//   - "config/*" - matches all files directly in the config directory
//   - "data/**/*.txt" - matches all .txt files in data and all subdirectories
//   - "bin/app" - matches exact file path
//
// WithFilesToExtract configures selective file extraction using glob patterns.
// When specified, only files matching at least one of the provided patterns will
// be extracted from the archive. This saves disk I/O and CPU time when you only
// need a subset of files from a large bundle.
//
// Glob pattern syntax:
//   - "*" matches any sequence of characters (excluding directory separator)
//   - "?" matches any single character (excluding directory separator)
//   - "**" matches any sequence of characters (including directory separators)
//   - Character ranges like [a-z] and [0-9] are supported
//
// Pattern examples:
//   - "*.json" - matches all .json files in the root directory only
//   - "config.json" - matches only the specific file config.json in root
//   - "config/*" - matches all files directly under config/ directory
//   - "**/*.json" - matches all .json files recursively in any directory
//   - "data/**/*.txt" - matches all .txt files under data/ and its subdirectories
//   - "src/**/*.go" - matches all .go files under src/ and subdirectories
//
// Multiple patterns can be provided, and a file will be extracted if it matches
// ANY of the patterns (logical OR).
//
// Security notes:
//   - All security validators still apply to matched files
//   - Size limits, file count limits, and path traversal checks are enforced
//   - Directories needed for matched files are created automatically
//   - Non-matching files are completely skipped (not counted toward limits)
//
// Performance:
//   - Current implementation downloads the full archive but skips non-matching files
//   - Saves disk I/O: non-matching files are never written to disk
//   - Saves CPU: non-matching files are not decompressed or validated
//   - Future: HTTP Range requests will minimize bandwidth usage
//
// Example - Extract only JSON configuration files:
//
//	err := client.Pull(ctx, ref, targetDir,
//	    ocibundle.WithFilesToExtract("**/*.json"),
//	)
//
// Example - Extract configuration and data files:
//
//	err := client.Pull(ctx, ref, targetDir,
//	    ocibundle.WithFilesToExtract("config.json", "data/*.json", "secrets/*.yaml"),
//	)
//
// Example - Extract all source code:
//
//	err := client.Pull(ctx, ref, targetDir,
//	    ocibundle.WithFilesToExtract("**/*.go", "**/*.mod", "**/*.sum"),
//	)
//
// Example - Extract with security limits:
//
//	err := client.Pull(ctx, ref, targetDir,
//	    ocibundle.WithFilesToExtract("**/*.json"),
//	    ocibundle.WithMaxSize(10*1024*1024),  // 10MB total
//	    ocibundle.WithMaxFiles(100),          // Max 100 files
//	)
func WithFilesToExtract(patterns ...string) PullOption {
	return func(opts *PullOptions) {
		opts.FilesToExtract = patterns
	}
}

// WithMaxFiles is an alias for WithPullMaxFiles for convenience.
func WithMaxFiles(maxFiles int) PullOption {
	return WithPullMaxFiles(maxFiles)
}

// WithMaxSize is an alias for WithPullMaxSize for convenience.
func WithMaxSize(maxSize int64) PullOption {
	return WithPullMaxSize(maxSize)
}

// WithCacheBypass is an alias for WithPullCacheBypass for convenience.
func WithCacheBypass(bypass bool) PullOption {
	return WithPullCacheBypass(bypass)
}

// DefaultPullOptions returns the default pull options.
func DefaultPullOptions() *PullOptions {
	return &PullOptions{
		MaxFiles:            10000,
		MaxSize:             1 * 1024 * 1024 * 1024, // 1GB
		MaxFileSize:         100 * 1024 * 1024,      // 100MB
		AllowHiddenFiles:    false,
		PreservePermissions: false,
		StripPrefix:         "",
		MaxRetries:          3,
		RetryDelay:          2 * time.Second,
		CacheBypass:         false, // Use cache by default
		FilesToExtract:      nil,   // Extract all files by default
	}
}

// DefaultPushOptions returns the default push options.
func DefaultPushOptions() *PushOptions {
	return &PushOptions{
		Annotations:      make(map[string]string),
		Platform:         "",
		ProgressCallback: nil,
		MaxRetries:       3,
		RetryDelay:       2 * time.Second,
		CacheBypass:      false, // Use cache by default
	}
}

// DefaultClientOptions returns the default client options.
func DefaultClientOptions() *ClientOptions {
	return &ClientOptions{
		Auth:        nil, // Use default Docker credential chain
		HTTPConfig:  nil, // Use default HTTPS with certificate validation
		FS:          nil, // Filled by constructor if unset
		CacheConfig: nil, // Caching disabled by default
	}
}

// WithFilesystem injects a custom filesystem implementation used by the client.
func WithFilesystem(fsys core.FS) ClientOption {
	return func(opts *ClientOptions) {
		opts.FS = fsys
	}
}

// WithCache configures caching for OCI operations.
// The coordinator parameter provides the cache implementation.
// The cachePath parameter specifies where to store cache data.
// The maxSizeBytes parameter sets the maximum cache size (0 for default 1GB).
// The defaultTTL parameter sets the default TTL for cache entries (0 for default 24h).
func WithCache(coordinator cache.Cache, cachePath string, maxSizeBytes int64, defaultTTL time.Duration) ClientOption {
	return func(opts *ClientOptions) {
		if opts.CacheConfig == nil {
			opts.CacheConfig = &CacheConfig{}
		}
		opts.CacheConfig.Coordinator = coordinator
		opts.CacheConfig.CachePath = cachePath
		opts.CacheConfig.Policy = CachePolicyEnabled
		if maxSizeBytes > 0 {
			opts.CacheConfig.MaxSizeBytes = maxSizeBytes
		} else {
			opts.CacheConfig.MaxSizeBytes = 1024 * 1024 * 1024 // 1GB default
		}
		if defaultTTL > 0 {
			opts.CacheConfig.DefaultTTL = defaultTTL
		} else {
			opts.CacheConfig.DefaultTTL = 24 * time.Hour // 24 hours default
		}
	}
}

// WithCachePolicy sets the cache policy for OCI operations.
// The policy parameter controls when caching should be applied:
// - CachePolicyDisabled: No caching
// - CachePolicyEnabled: Cache all operations
// - CachePolicyPull: Cache only pull operations
// - CachePolicyPush: Cache only push operations
func WithCachePolicy(policy CachePolicy) ClientOption {
	return func(opts *ClientOptions) {
		if opts.CacheConfig == nil {
			opts.CacheConfig = &CacheConfig{}
		}
		opts.CacheConfig.Policy = policy
	}
}
