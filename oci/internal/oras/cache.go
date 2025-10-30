// Package oras provides caching functionality for HTTP transports and authentication credentials.
// This file contains optimized caching mechanisms for improved performance in OCI registry operations.
package oras

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"oras.land/oras-go/v2/registry/remote/auth"
)

// transportCache provides global caching of HTTP transports to optimize connection reuse.
// This improves performance by sharing connection pools across multiple repository instances.
type transportCache struct {
	transports map[string]http.RoundTripper
	mutex      sync.RWMutex
}

var globalTransportCache = &transportCache{
	transports: make(map[string]http.RoundTripper),
}

// authCache provides global caching of authentication credentials to reduce lookup overhead.
// This improves performance by avoiding repeated credential resolution for the same registry.
type authCache struct {
	credentials map[string]cachedCredential
	mutex       sync.RWMutex
}

type cachedCredential struct {
	credential auth.Credential
	expiresAt  time.Time
}

// Global authentication cache with 5-minute TTL for credentials
var globalAuthCache = &authCache{
	credentials: make(map[string]cachedCredential),
}

const authCacheTTL = 5 * time.Minute

// getCachedCredential retrieves a cached credential for the given registry.
// Returns nil if no cached credential exists or if it has expired.
func (ac *authCache) getCachedCredential(registry string) *auth.Credential {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()

	if cached, exists := ac.credentials[registry]; exists {
		if time.Now().Before(cached.expiresAt) {
			return &cached.credential
		}
		// Credential expired, remove it
		delete(ac.credentials, registry)
	}
	return nil
}

// setCachedCredential stores a credential in the cache with TTL.
func (ac *authCache) setCachedCredential(registry string, credential auth.Credential) {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()

	ac.credentials[registry] = cachedCredential{
		credential: credential,
		expiresAt:  time.Now().Add(authCacheTTL),
	}
}

// ClearAuthCache clears all cached authentication credentials.
// This is primarily intended for testing to ensure test isolation.
func ClearAuthCache() {
	globalAuthCache.mutex.Lock()
	defer globalAuthCache.mutex.Unlock()

	// Clear the map by reassigning
	globalAuthCache.credentials = make(map[string]cachedCredential)
}

// getCachedTransport retrieves a cached transport for the given configuration key.
// Returns nil if no cached transport exists for the key.
func (tc *transportCache) getCachedTransport(key string) http.RoundTripper {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()
	return tc.transports[key]
}

// setCachedTransport stores a transport in the cache with the given configuration key.
func (tc *transportCache) setCachedTransport(key string, transport http.RoundTripper) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	tc.transports[key] = transport
}

// generateTransportKey creates a unique key for transport caching based on configuration.
func generateTransportKey(opts *AuthOptions) string {
	if opts == nil {
		return "default"
	}

	var keyParts []string

	// Include HTTP config in key
	if opts.HTTPConfig != nil {
		if opts.HTTPConfig.AllowHTTP {
			keyParts = append(keyParts, "http")
		} else {
			keyParts = append(keyParts, "https")
		}
		if opts.HTTPConfig.AllowInsecure {
			keyParts = append(keyParts, "insecure")
		}
		keyParts = append(keyParts, fmt.Sprintf("registries:%v", opts.HTTPConfig.Registries))
	}

	// Include static auth registry in key (but not credentials for security)
	if opts.StaticRegistry != "" {
		keyParts = append(keyParts, fmt.Sprintf("static:%s", opts.StaticRegistry))
	}

	if len(keyParts) == 0 {
		return "default"
	}

	return strings.Join(keyParts, "|")
}

// newDefaultTransport creates an HTTP transport with optimized connection pooling.
// This provides better performance by reusing connections and managing connection limits.
// Uses global caching to share transports across repository instances.
func newDefaultTransport(opts *AuthOptions) http.RoundTripper {
	// If a custom transport is provided, use it (don't cache)
	if opts != nil && opts.Transport != nil {
		return opts.Transport
	}

	// Generate cache key for this configuration
	cacheKey := generateTransportKey(opts)

	// Check cache first
	if cachedTransport := globalTransportCache.getCachedTransport(cacheKey); cachedTransport != nil {
		return cachedTransport
	}

	// Create transport with connection pooling optimized for OCI registries
	transport := &http.Transport{
		// Connection pooling settings optimized for OCI registry operations
		MaxIdleConns:        100,              // Maximum idle connections across all hosts
		MaxIdleConnsPerHost: 10,               // Maximum idle connections per host
		MaxConnsPerHost:     20,               // Maximum connections per host (including active)
		IdleConnTimeout:     90 * time.Second, // How long to keep idle connections alive

		// Timeout settings for registry operations
		ResponseHeaderTimeout: 30 * time.Second, // Time to wait for response headers
		ExpectContinueTimeout: 1 * time.Second,  // Time to wait for 100-continue response

		// Keep alive settings for connection reuse
		DisableKeepAlives: false, // Enable keep-alive connections

		// TLS settings
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Apply HTTP config settings if provided
	if opts != nil && opts.HTTPConfig != nil {
		// Configure TLS settings for insecure connections
		if opts.HTTPConfig.AllowInsecure {
			transport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true, // Allow self-signed certificates
			}
		}
	}

	// Cache the transport for future use
	globalTransportCache.setCachedTransport(cacheKey, transport)

	return transport
}

// newCachedCredentialFunc wraps a credential function with caching to reduce authentication overhead.
// It caches successful credential lookups for 5 minutes to improve performance.
func newCachedCredentialFunc(baseFunc auth.CredentialFunc) auth.CredentialFunc {
	if baseFunc == nil {
		return nil
	}

	return func(ctx context.Context, registry string) (auth.Credential, error) {
		// Check cache first
		if cachedCred := globalAuthCache.getCachedCredential(registry); cachedCred != nil {
			return *cachedCred, nil
		}

		// Cache miss - call the base function
		cred, err := baseFunc(ctx, registry)
		if err != nil {
			return auth.Credential{}, err
		}

		// Cache successful result
		globalAuthCache.setCachedCredential(registry, cred)

		return cred, nil
	}
}
