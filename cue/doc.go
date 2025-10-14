/*
Package cue provides CUE evaluation and validation capabilities.

# Overview

This library wraps cuelang.org/go functionality with platform-specific error handling,
filesystem abstraction, and extensible attribute processing infrastructure. It serves as
Layer 2 infrastructure, providing the toolkit for CUE operations without implementing
business logic.

The library is designed to be generic and reusable - it works with any CUE schemas and
any Go struct types, without coupling to specific schema or configuration packages.

# Architecture

The package is organized into several components:

  - Loader: Load CUE modules, packages, and files from filesystem
  - Validator: Validate CUE values against schemas
  - Encoder: Encode CUE values to YAML/JSON
  - Decoder: Decode CUE values to Go structs
  - Attributes: Extensible attribute processing infrastructure (sub-package)

# Caller Responsibilities

This library is designed to be stateless and flexible, leaving several responsibilities
to the caller:

  - CUE Context Management: The Loader manages its own CUE context, but callers can
    access it via Context() for advanced operations
  - Filesystem Abstraction: All file operations use fs/core.ReadFS interface
  - Caching: No built-in caching - implement at caller level if needed
  - Timeouts: Use context.WithTimeout() for operation time limits
  - Attribute Processors: Register custom processors via attributes.Registry

# Example 1: Load, Validate, and Decode Config

This example demonstrates the common pattern of loading a CUE configuration file,
validating it against a schema, and decoding it into a Go struct.

	package main

	import (
		"context"
		"fmt"
		"log"

		"github.com/jmgilman/go/cue"
		"github.com/jmgilman/go/fs/billy"
		"github.com/jmgilman/go/fs/core"
	)

	// Define your configuration struct
	type AppConfig struct {
		Name    string `json:"name"`
		Port    int    `json:"port"`
		Debug   bool   `json:"debug"`
		Timeout string `json:"timeout"`
	}

	func LoadAndValidateConfig(schemaFS, configFS core.ReadFS) (*AppConfig, error) {
		ctx := context.Background()

		// Create loaders for schema and config
		schemaLoader := cue.NewLoader(schemaFS)
		configLoader := cue.NewLoader(configFS)

		// Load schema module
		schema, err := schemaLoader.LoadModule(ctx, "schema")
		if err != nil {
			return nil, fmt.Errorf("failed to load schema: %w", err)
		}

		// Load config file
		configValue, err := configLoader.LoadFile(ctx, "config.cue")
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}

		// Validate config against schema
		if err := cue.Validate(ctx, schema, configValue); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}

		// Decode to Go struct
		var config AppConfig
		if err := cue.Decode(ctx, configValue, &config); err != nil {
			return nil, fmt.Errorf("decode failed: %w", err)
		}

		return &config, nil
	}

	func main() {
		// Create filesystem instances (example uses billy wrapper)
		schemaFS := billy.NewReadOnlyFS("/path/to/schemas")
		configFS := billy.NewReadOnlyFS("/path/to/configs")

		config, err := LoadAndValidateConfig(schemaFS, configFS)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		fmt.Printf("Loaded config: %+v\n", config)
	}

# Example 2: Attribute Processing

This example shows how to implement custom attribute processors for runtime value
substitution. This is useful for processing deployment manifests where values need
to be resolved at runtime (e.g., artifact URIs, secrets, etc.).

	package main

	import (
		"context"
		"fmt"
		"log"

		"cuelang.org/go/cue"
		"github.com/jmgilman/go/cue"
		cueattrs "github.com/jmgilman/go/cue/attributes"
		"github.com/jmgilman/go/fs/billy"
	)

	// Domain model (would come from your application)
	type Artifact struct {
		Name      string
		OciURI    string
		OciDigest string
	}

	type Release struct {
		Version   string
		Artifacts []Artifact
	}

	// Custom processor implementation
	type ArtifactProcessor struct {
		cueCtx  *cue.Context
		release *Release
	}

	func (p *ArtifactProcessor) Name() string {
		return "artifact"
	}

	func (p *ArtifactProcessor) Process(ctx context.Context, attr cueattrs.Attribute) (cue.Value, error) {
		// Extract arguments from the attribute
		name := attr.Args["name"]
		field := attr.Args["field"]

		// Look up artifact in release
		for _, artifact := range p.release.Artifacts {
			if artifact.Name == name {
				switch field {
				case "uri":
					return p.cueCtx.CompileString(fmt.Sprintf("%q", artifact.OciURI)), nil
				case "digest":
					return p.cueCtx.CompileString(fmt.Sprintf("%q", artifact.OciDigest)), nil
				}
			}
		}

		return cue.Value{}, fmt.Errorf("artifact %q not found", name)
	}

	func ProcessDeploymentManifest(deploymentPath string, release *Release) ([]byte, error) {
		ctx := context.Background()

		// Load the deployment CUE file
		fs := billy.NewReadOnlyFS("/path/to/deployments")
		loader := cue.NewLoader(fs)

		value, err := loader.LoadFile(ctx, deploymentPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load deployment: %w", err)
		}

		// Create and register the custom attribute handler
		processor := &ArtifactProcessor{
			cueCtx:  loader.Context(),
			release: release,
		}
		registry := cueattrs.NewRegistry()
		if err := registry.Register(processor); err != nil {
			return nil, fmt.Errorf("failed to register processor: %w", err)
		}

		// Walk and process attributes
		walker := cueattrs.NewWalker(registry, loader.Context())
		result, err := walker.Walk(ctx, value)
		if err != nil {
			return nil, fmt.Errorf("attribute processing failed: %w", err)
		}

		// Encode final result to YAML
		return cue.EncodeYAML(ctx, result)
	}

	func main() {
		// Example release data
		release := &Release{
			Version: "v1.0.0",
			Artifacts: []Artifact{
				{
					Name:      "api-server",
					OciURI:    "ghcr.io/myorg/api-server:v1.0.0",
					OciDigest: "sha256:abc123...",
				},
				{
					Name:      "worker",
					OciURI:    "ghcr.io/myorg/worker:v1.0.0",
					OciDigest: "sha256:def456...",
				},
			},
		}

		yaml, err := ProcessDeploymentManifest("deployment.cue", release)
		if err != nil {
			log.Fatalf("Failed to process deployment: %v", err)
		}

		fmt.Println(string(yaml))
	}

Example deployment.cue file that would be processed:

	deployment: {
		apiServer: {
			image: _ @artifact(name="api-server", field="uri")
			digest: _ @artifact(name="api-server", field="digest")
		}
		worker: {
			image: _ @artifact(name="worker", field="uri")
			digest: _ @artifact(name="worker", field="digest")
		}
	}

# Example 3: Streaming Large Manifests

For large YAML outputs (e.g., Kubernetes manifests), use streaming to avoid loading
the entire result into memory.

	package main

	import (
		"context"
		"fmt"
		"log"
		"os"

		"github.com/jmgilman/go/cue"
		"github.com/jmgilman/go/fs/billy"
	)

	func RenderLargeManifest(manifestPath, outputPath string) error {
		ctx := context.Background()

		// Load the CUE manifest
		fs := billy.NewReadOnlyFS("/path/to/manifests")
		loader := cue.NewLoader(fs)

		value, err := loader.LoadModule(ctx, manifestPath)
		if err != nil {
			return fmt.Errorf("failed to load manifest: %w", err)
		}

		// Open output file
		file, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()

		// Stream YAML to file (avoids loading all into memory)
		if err := cue.EncodeYAMLStream(ctx, value, file); err != nil {
			return fmt.Errorf("failed to encode manifest: %w", err)
		}

		return nil
	}

	func main() {
		if err := RenderLargeManifest("k8s/manifests", "output.yaml"); err != nil {
			log.Fatalf("Failed to render manifest: %v", err)
		}
		fmt.Println("Manifest rendered successfully")
	}

# Public API Surface

Main package functions:

	// Loading
	func NewLoader(filesystem core.ReadFS) *Loader
	func (l *Loader) LoadFile(ctx context.Context, filePath string) (cue.Value, error)
	func (l *Loader) LoadPackage(ctx context.Context, packagePath string) (cue.Value, error)
	func (l *Loader) LoadModule(ctx context.Context, modulePath string) (cue.Value, error)
	func (l *Loader) LoadBytes(ctx context.Context, source []byte, filename string) (cue.Value, error)
	func (l *Loader) Context() *cue.Context

	// Validation
	func Validate(ctx context.Context, schema cue.Value, data cue.Value) error
	func ValidateWithOptions(ctx context.Context, schema cue.Value, data cue.Value, opts ValidationOptions) error
	func ValidateConstraint(ctx context.Context, value cue.Value, constraint cue.Value) error
	func ValidateConstraintWithOptions(ctx context.Context, value cue.Value, constraint cue.Value, opts ValidationOptions) error

	// Encoding
	func EncodeYAML(ctx context.Context, value cue.Value) ([]byte, errors.PlatformError)
	func EncodeJSON(ctx context.Context, value cue.Value) ([]byte, errors.PlatformError)
	func EncodeYAMLStream(ctx context.Context, value cue.Value, w io.Writer) errors.PlatformError

	// Decoding
	func Decode(ctx context.Context, value cue.Value, target interface{}) errors.PlatformError

Attributes sub-package (cue/attributes):

	type Processor interface {
		Name() string
		Process(ctx context.Context, attr Attribute) (cue.Value, error)
	}

	type Attribute struct {
		Name  string
		Args  map[string]string
		Path  cue.Path
		Value cue.Value
	}

	type Registry struct { ... }
	func NewRegistry() *Registry
	func (r *Registry) Register(p Processor) error
	func (r *Registry) Get(name string) (Processor, bool)

	type Walker struct { ... }
	func NewWalker(registry *Registry, cueCtx *cue.Context) *Walker
	func (w *Walker) Walk(ctx context.Context, value cue.Value) (cue.Value, error)

	func ParseAttribute(value cue.Value, attrName string) (Attribute, bool)
	func ParseArgs(attrText string) (map[string]string, error)

# Error Handling

All errors are wrapped with errors.PlatformError, providing consistent error handling
across the platform. Error codes include:

  - CodeCUELoadFailed: Module/file loading failures
  - CodeCUEBuildFailed: Build/evaluation failures
  - CodeCUEValidationFailed: Validation failures
  - CodeCUEDecodeFailed: Decoding failures
  - CodeCUEEncodeFailed: Encoding failures

Validation errors include detailed field path information and structured error messages
for debugging.

# Related Packages

  - github.com/jmgilman/go/errors - Platform error handling
  - github.com/jmgilman/go/fs/core - Filesystem abstraction interfaces
  - github.com/jmgilman/go/fs/billy - Billy filesystem wrapper
  - cuelang.org/go - CUE language implementation

# Performance Considerations

  - Module loading can be expensive - caller should cache cue.Value results if needed
  - Use EncodeYAMLStream() for large manifests (>10MB) to avoid memory pressure
  - CUE validation is typically fast (<100ms) but complex schemas may take longer
  - Use context.WithTimeout() to set time limits on operations
*/
package cue
