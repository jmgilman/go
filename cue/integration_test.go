package cue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/jmgilman/go/cue/attributes"
	platformerrors "github.com/jmgilman/go/errors"
	"github.com/jmgilman/go/fs/billy"
)

// TestIntegration_LoadValidateDecode tests the complete workflow of loading,
// validating, and decoding a CUE configuration.
func TestIntegration_LoadValidateDecode(t *testing.T) {
	t.Run("complete workflow with valid config", func(t *testing.T) {
		// Setup in-memory filesystem with a schema and config
		mfs := billy.NewMemory()

		// Create a schema file
		schemaContent := `
package config

#Config: {
	name:    string
	version: string & =~"^[0-9]+\\.[0-9]+\\.[0-9]+$"
	port:    int & >0 & <65536
	enabled: bool
}
`
		if err := mfs.WriteFile("schema.cue", []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to create schema file: %v", err)
		}

		// Create a config file that conforms to the schema
		configContent := `
package config

config: #Config & {
	name:    "my-service"
	version: "1.2.3"
	port:    8080
	enabled: true
}
`
		if err := mfs.WriteFile("config.cue", []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		// Define Go struct for decoding
		type Config struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Port    int    `json:"port"`
			Enabled bool   `json:"enabled"`
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		// Step 1: Load the module
		value, err := loader.LoadModule(ctx, ".")
		if err != nil {
			t.Fatalf("LoadModule failed: %v", err)
		}

		// Step 2: Extract schema and config
		schemaVal := value.LookupPath(cue.ParsePath("#Config"))
		if schemaVal.Err() != nil {
			t.Fatalf("failed to lookup schema: %v", schemaVal.Err())
		}

		configVal := value.LookupPath(cue.ParsePath("config"))
		if configVal.Err() != nil {
			t.Fatalf("failed to lookup config: %v", configVal.Err())
		}

		// Step 3: Validate config against schema
		if err := Validate(ctx, schemaVal, configVal); err != nil {
			t.Fatalf("Validate failed: %v", err)
		}

		// Step 4: Decode to Go struct
		var config Config
		if err := Decode(ctx, configVal, &config); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}

		// Verify the decoded values
		if config.Name != "my-service" {
			t.Errorf("expected name='my-service', got %q", config.Name)
		}
		if config.Version != "1.2.3" {
			t.Errorf("expected version='1.2.3', got %q", config.Version)
		}
		if config.Port != 8080 {
			t.Errorf("expected port=8080, got %d", config.Port)
		}
		if !config.Enabled {
			t.Error("expected enabled=true, got false")
		}
	})

	t.Run("validation fails with invalid config", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create a schema file
		schemaContent := `
package config

#Config: {
	port: int & >0 & <65536
}
`
		if err := mfs.WriteFile("schema.cue", []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to create schema file: %v", err)
		}

		// Create a config that doesn't explicitly apply the constraint yet
		configContent := `
package config

config: {
	port: 99999
}
`
		if err := mfs.WriteFile("config.cue", []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadModule(ctx, ".")
		if err != nil {
			t.Fatalf("LoadModule failed: %v", err)
		}

		schemaVal := value.LookupPath(cue.ParsePath("#Config"))
		configVal := value.LookupPath(cue.ParsePath("config"))

		// Validation should fail when we apply the schema constraint
		err = Validate(ctx, schemaVal, configVal)
		if err == nil {
			t.Fatal("expected validation to fail, got nil")
		}

		// Check error code
		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEValidationFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, perr.Code())
		}
	})
}

// TestIntegration_LoadValidateEncodeYAML tests loading CUE files, validating,
// and encoding to YAML format.
func TestIntegration_LoadValidateEncodeYAML(t *testing.T) {
	t.Run("load and encode deployment to YAML", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create a deployment CUE file
		deploymentContent := `
package deployment

deployment: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: "nginx"
		namespace: "default"
	}
	spec: {
		replicas: 3
		selector: matchLabels: app: "nginx"
		template: {
			metadata: labels: app: "nginx"
			spec: containers: [{
				name: "nginx"
				image: "nginx:1.14.2"
				ports: [{
					containerPort: 80
				}]
			}]
		}
	}
}
`
		if err := mfs.WriteFile("deployment.cue", []byte(deploymentContent), 0644); err != nil {
			t.Fatalf("failed to create deployment file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		// Load the file
		value, err := loader.LoadFile(ctx, "deployment.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		// Extract deployment
		deploymentVal := value.LookupPath(cue.ParsePath("deployment"))
		if deploymentVal.Err() != nil {
			t.Fatalf("failed to lookup deployment: %v", deploymentVal.Err())
		}

		// Encode to YAML
		yamlBytes, err := EncodeYAML(ctx, deploymentVal)
		if err != nil {
			t.Fatalf("EncodeYAML failed: %v", err)
		}

		// Verify YAML contains expected content
		yamlStr := string(yamlBytes)
		expectedStrings := []string{
			"apiVersion: apps/v1",
			"kind: Deployment",
			"name: nginx",
			"replicas: 3",
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(yamlStr, expected) {
				t.Errorf("expected YAML to contain %q, but it didn't", expected)
			}
		}
	})

	t.Run("encode JSON from CUE config", func(t *testing.T) {
		mfs := billy.NewMemory()

		configContent := `
package config

settings: {
	database: {
		host: "localhost"
		port: 5432
		name: "mydb"
	}
	cache: {
		enabled: true
		ttl: 300
	}
}
`
		if err := mfs.WriteFile("config.cue", []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadFile(ctx, "config.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		settingsVal := value.LookupPath(cue.ParsePath("settings"))
		if settingsVal.Err() != nil {
			t.Fatalf("failed to lookup settings: %v", settingsVal.Err())
		}

		// Encode to JSON
		jsonBytes, err := EncodeJSON(ctx, settingsVal)
		if err != nil {
			t.Fatalf("EncodeJSON failed: %v", err)
		}

		// Verify JSON is valid and contains expected content
		jsonStr := string(jsonBytes)
		expectedStrings := []string{
			`"database"`,
			`"host":"localhost"`,
			`"port":5432`,
			`"cache"`,
			`"enabled":true`,
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(jsonStr, expected) {
				t.Errorf("expected JSON to contain %q, but it didn't", expected)
			}
		}
	})
}

// mockArtifactProcessor is a mock processor for testing attribute processing.
type mockArtifactProcessor struct {
	cueCtx    *cue.Context
	artifacts map[string]map[string]string
}

func newMockArtifactProcessor(ctx *cue.Context) *mockArtifactProcessor {
	return &mockArtifactProcessor{
		cueCtx: ctx,
		artifacts: map[string]map[string]string{
			"api-server": {
				"uri":    "ghcr.io/example/api-server:v1.0.0",
				"digest": "sha256:abc123",
			},
			"worker": {
				"uri":    "ghcr.io/example/worker:v1.0.0",
				"digest": "sha256:def456",
			},
		},
	}
}

func (p *mockArtifactProcessor) Name() string {
	return "artifact"
}

func (p *mockArtifactProcessor) Process(_ context.Context, attr attributes.Attribute) (cue.Value, error) {
	name, ok := attr.Args["name"]
	if !ok {
		return cue.Value{}, fmt.Errorf("missing 'name' argument")
	}

	field, ok := attr.Args["field"]
	if !ok {
		return cue.Value{}, fmt.Errorf("missing 'field' argument")
	}

	artifact, ok := p.artifacts[name]
	if !ok {
		return cue.Value{}, fmt.Errorf("artifact %q not found", name)
	}

	value, ok := artifact[field]
	if !ok {
		return cue.Value{}, fmt.Errorf("field %q not found in artifact %q", field, name)
	}

	// Return the value as a CUE string
	return p.cueCtx.CompileString(fmt.Sprintf(`"%s"`, value)), nil
}

// TestIntegration_AttributeProcessing tests attribute processing with a mock processor.
func TestIntegration_AttributeProcessing(t *testing.T) {
	t.Run("process attributes in deployment", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create a deployment with artifact attributes
		deploymentContent := `
package deployment

deployment: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: "api-server"
	}
	spec: {
		template: {
			spec: {
				containers: [{
					name: "api-server"
					image: "placeholder" @artifact(name="api-server", field="uri")
				}]
			}
		}
	}
}
`
		if err := mfs.WriteFile("deployment.cue", []byte(deploymentContent), 0644); err != nil {
			t.Fatalf("failed to create deployment file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadFile(ctx, "deployment.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		deploymentVal := value.LookupPath(cue.ParsePath("deployment"))
		if deploymentVal.Err() != nil {
			t.Fatalf("failed to lookup deployment: %v", deploymentVal.Err())
		}

		// Create and register mock processor
		processor := newMockArtifactProcessor(loader.Context())
		registry := attributes.NewRegistry()
		if err := registry.Register(processor); err != nil {
			t.Fatalf("failed to register processor: %v", err)
		}

		// Walk and process attributes
		walker := attributes.NewWalker(registry, loader.Context())
		result, err := walker.Walk(ctx, deploymentVal)
		if err != nil {
			t.Fatalf("Walk failed: %v", err)
		}

		// Encode to YAML to verify the result
		yamlBytes, err := EncodeYAML(ctx, result)
		if err != nil {
			t.Fatalf("EncodeYAML failed: %v", err)
		}

		yamlStr := string(yamlBytes)

		// Verify the attribute was replaced with the actual value
		if !strings.Contains(yamlStr, "ghcr.io/example/api-server:v1.0.0") {
			t.Errorf("expected YAML to contain artifact URI, got:\n%s", yamlStr)
		}

		// Verify placeholder is gone
		if strings.Contains(yamlStr, "placeholder") {
			t.Errorf("expected placeholder to be replaced, got:\n%s", yamlStr)
		}
	})

	t.Run("process multiple artifacts", func(t *testing.T) {
		mfs := billy.NewMemory()

		deploymentContent := `
package deployment

services: {
	api: {
		image: "placeholder" @artifact(name="api-server", field="uri")
		digest: "placeholder" @artifact(name="api-server", field="digest")
	}
	worker: {
		image: "placeholder" @artifact(name="worker", field="uri")
		digest: "placeholder" @artifact(name="worker", field="digest")
	}
}
`
		if err := mfs.WriteFile("services.cue", []byte(deploymentContent), 0644); err != nil {
			t.Fatalf("failed to create services file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadFile(ctx, "services.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		servicesVal := value.LookupPath(cue.ParsePath("services"))
		if servicesVal.Err() != nil {
			t.Fatalf("failed to lookup services: %v", servicesVal.Err())
		}

		// Create and register mock processor
		processor := newMockArtifactProcessor(loader.Context())
		registry := attributes.NewRegistry()
		if err := registry.Register(processor); err != nil {
			t.Fatalf("failed to register processor: %v", err)
		}

		// Walk and process attributes
		walker := attributes.NewWalker(registry, loader.Context())
		result, err := walker.Walk(ctx, servicesVal)
		if err != nil {
			t.Fatalf("Walk failed: %v", err)
		}

		// Encode to JSON for verification
		jsonBytes, err := EncodeJSON(ctx, result)
		if err != nil {
			t.Fatalf("EncodeJSON failed: %v", err)
		}

		jsonStr := string(jsonBytes)

		// Verify all artifacts were replaced
		expectedValues := []string{
			"ghcr.io/example/api-server:v1.0.0",
			"sha256:abc123",
			"ghcr.io/example/worker:v1.0.0",
			"sha256:def456",
		}

		for _, expected := range expectedValues {
			if !strings.Contains(jsonStr, expected) {
				t.Errorf("expected JSON to contain %q, got:\n%s", expected, jsonStr)
			}
		}
	})

	t.Run("unknown attributes are ignored", func(t *testing.T) {
		mfs := billy.NewMemory()

		configContent := `
package config

value: "test" @unknown(arg="value") @artifact(name="api-server", field="uri")
`
		if err := mfs.WriteFile("config.cue", []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadFile(ctx, "config.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		valueField := value.LookupPath(cue.ParsePath("value"))
		if valueField.Err() != nil {
			t.Fatalf("failed to lookup value: %v", valueField.Err())
		}

		// Register only artifact processor (not unknown)
		processor := newMockArtifactProcessor(loader.Context())
		registry := attributes.NewRegistry()
		if err := registry.Register(processor); err != nil {
			t.Fatalf("failed to register processor: %v", err)
		}

		// Walk should succeed and ignore unknown attribute
		walker := attributes.NewWalker(registry, loader.Context())
		result, err := walker.Walk(ctx, valueField)
		if err != nil {
			t.Fatalf("Walk failed: %v", err)
		}

		// Result should be the artifact value (unknown was ignored)
		var str string
		if err := result.Decode(&str); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}

		if str != "ghcr.io/example/api-server:v1.0.0" {
			t.Errorf("expected artifact value, got %q", str)
		}
	})
}

// TestIntegration_ErrorMessagesWithFieldPaths tests that validation errors
// include field paths in the error context.
func TestIntegration_ErrorMessagesWithFieldPaths(t *testing.T) {
	t.Run("validation error includes field paths", func(t *testing.T) {
		mfs := billy.NewMemory()

		schemaContent := `
package config

#Config: {
	database: {
		host: string
		port: int & >0 & <65536
	}
	cache: {
		ttl: int & >=0
	}
}
`
		if err := mfs.WriteFile("schema.cue", []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to create schema file: %v", err)
		}

		// Config without schema constraint applied yet
		configContent := `
package config

config: {
	database: {
		host: "localhost"
		port: 99999
	}
	cache: {
		ttl: -1
	}
}
`
		if err := mfs.WriteFile("config.cue", []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadModule(ctx, ".")
		if err != nil {
			t.Fatalf("LoadModule failed: %v", err)
		}

		schemaVal := value.LookupPath(cue.ParsePath("#Config"))
		configVal := value.LookupPath(cue.ParsePath("config"))

		// Validation should fail
		err = Validate(ctx, schemaVal, configVal)
		if err == nil {
			t.Fatal("expected validation to fail, got nil")
		}

		// Check that error includes field path information
		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		errCtx := perr.Context()
		if errCtx == nil {
			t.Fatal("expected error context, got nil")
		}

		// Verify that issues are included in context
		issues, ok := errCtx["issues"]
		if !ok {
			t.Fatal("expected 'issues' in error context")
		}

		// The exact structure depends on the implementation, but we should have issues
		if issues == nil {
			t.Error("expected non-nil issues")
		}
	})
}

// TestIntegration_StreamEncoding tests streaming YAML encoding for large outputs.
func TestIntegration_StreamEncoding(t *testing.T) {
	t.Run("stream large manifest to writer", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create a CUE file with a list of items (simulating a large manifest)
		manifestContent := `
package manifest

items: [
	{name: "item-1", value: 100},
	{name: "item-2", value: 200},
	{name: "item-3", value: 300},
	{name: "item-4", value: 400},
	{name: "item-5", value: 500},
]
`
		if err := mfs.WriteFile("manifest.cue", []byte(manifestContent), 0644); err != nil {
			t.Fatalf("failed to create manifest file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadFile(ctx, "manifest.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		itemsVal := value.LookupPath(cue.ParsePath("items"))
		if itemsVal.Err() != nil {
			t.Fatalf("failed to lookup items: %v", itemsVal.Err())
		}

		// Stream to a buffer
		var buf bytes.Buffer
		if err := EncodeYAMLStream(ctx, itemsVal, &buf); err != nil {
			t.Fatalf("EncodeYAMLStream failed: %v", err)
		}

		// Verify the output
		yamlStr := buf.String()

		expectedStrings := []string{
			"name: item-1",
			"value: 100",
			"name: item-5",
			"value: 500",
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(yamlStr, expected) {
				t.Errorf("expected YAML to contain %q, but it didn't", expected)
			}
		}
	})

	t.Run("stream encoding writes all bytes", func(t *testing.T) {
		mfs := billy.NewMemory()

		simpleContent := `
package test

data: {
	key1: "value1"
	key2: "value2"
}
`
		if err := mfs.WriteFile("simple.cue", []byte(simpleContent), 0644); err != nil {
			t.Fatalf("failed to create simple file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadFile(ctx, "simple.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		dataVal := value.LookupPath(cue.ParsePath("data"))
		if dataVal.Err() != nil {
			t.Fatalf("failed to lookup data: %v", dataVal.Err())
		}

		// First get the size via regular encoding
		regularBytes, err := EncodeYAML(ctx, dataVal)
		if err != nil {
			t.Fatalf("EncodeYAML failed: %v", err)
		}

		// Now stream to buffer
		var buf bytes.Buffer
		if err := EncodeYAMLStream(ctx, dataVal, &buf); err != nil {
			t.Fatalf("EncodeYAMLStream failed: %v", err)
		}

		// Verify same content
		if buf.Len() != len(regularBytes) {
			t.Errorf("expected %d bytes, got %d bytes", len(regularBytes), buf.Len())
		}

		if buf.String() != string(regularBytes) {
			t.Error("stream output differs from regular encoding")
		}
	})
}

// TestIntegration_CompleteWorkflow tests a complete realistic workflow
// combining multiple operations.
func TestIntegration_CompleteWorkflow(t *testing.T) {
	t.Run("load schema, validate config, process attributes, encode YAML", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create a deployment schema
		schemaContent := `
package deployment

#Deployment: {
	name: string
	replicas: int & >0
	image: string
}
`
		if err := mfs.WriteFile("schema.cue", []byte(schemaContent), 0644); err != nil {
			t.Fatalf("failed to create schema file: %v", err)
		}

		// Create a deployment that uses artifact attributes
		deploymentContent := `
package deployment

myDeployment: #Deployment & {
	name: "api-server"
	replicas: 3
	image: "placeholder" @artifact(name="api-server", field="uri")
}
`
		if err := mfs.WriteFile("deployment.cue", []byte(deploymentContent), 0644); err != nil {
			t.Fatalf("failed to create deployment file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		// Step 1: Load module
		value, err := loader.LoadModule(ctx, ".")
		if err != nil {
			t.Fatalf("LoadModule failed: %v", err)
		}

		// Step 2: Validate deployment against schema
		schemaVal := value.LookupPath(cue.ParsePath("#Deployment"))
		deploymentVal := value.LookupPath(cue.ParsePath("myDeployment"))

		if err := Validate(ctx, schemaVal, deploymentVal); err != nil {
			t.Fatalf("Validate failed: %v", err)
		}

		// Step 3: Process attributes
		processor := newMockArtifactProcessor(loader.Context())
		registry := attributes.NewRegistry()
		if err := registry.Register(processor); err != nil {
			t.Fatalf("failed to register processor: %v", err)
		}

		walker := attributes.NewWalker(registry, loader.Context())
		processedVal, err := walker.Walk(ctx, deploymentVal)
		if err != nil {
			t.Fatalf("Walk failed: %v", err)
		}

		// Step 4: Encode to YAML
		yamlBytes, err := EncodeYAML(ctx, processedVal)
		if err != nil {
			t.Fatalf("EncodeYAML failed: %v", err)
		}

		yamlStr := string(yamlBytes)

		// Verify final output
		if !strings.Contains(yamlStr, "name: api-server") {
			t.Error("expected YAML to contain name")
		}
		if !strings.Contains(yamlStr, "replicas: 3") {
			t.Error("expected YAML to contain replicas")
		}
		if !strings.Contains(yamlStr, "ghcr.io/example/api-server:v1.0.0") {
			t.Error("expected YAML to contain processed artifact URI")
		}
		if strings.Contains(yamlStr, "placeholder") {
			t.Error("expected placeholder to be replaced")
		}
	})
}

// TestIntegration_ErrorPaths tests that various error scenarios produce
// appropriate error codes and context.
func TestIntegration_ErrorPaths(t *testing.T) {
	t.Run("load error produces CodeCUELoadFailed", func(t *testing.T) {
		mfs := billy.NewMemory()
		// Empty filesystem

		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadModule(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}
	})

	t.Run("build error produces CodeCUEBuildFailed", func(t *testing.T) {
		mfs := billy.NewMemory()

		invalidContent := `
package test
this is invalid syntax
`
		if err := mfs.WriteFile("invalid.cue", []byte(invalidContent), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadFile(ctx, "invalid.cue")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEBuildFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEBuildFailed, perr.Code())
		}
	})

	t.Run("validation error produces CodeCUEValidationFailed", func(t *testing.T) {
		mfs := billy.NewMemory()

		content := `
package test

#Schema: {
	value: int & >0
}

data: {
	value: -1
}
`
		if err := mfs.WriteFile("test.cue", []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		value, err := loader.LoadFile(ctx, "test.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		schemaVal := value.LookupPath(cue.ParsePath("#Schema"))
		dataVal := value.LookupPath(cue.ParsePath("data"))

		err = Validate(ctx, schemaVal, dataVal)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEValidationFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEValidationFailed, perr.Code())
		}
	})

	t.Run("decode error produces CodeCUEDecodeFailed", func(t *testing.T) {
		ctx := context.Background()
		loader := NewLoader(nil)

		value := loader.Context().CompileString(`{name: string}`) // incomplete

		var target struct {
			Name string
		}

		err := Decode(ctx, value, &target)
		if err == nil {
			t.Fatal("expected decode error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEDecodeFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEDecodeFailed, perr.Code())
		}
	})

	t.Run("encode error produces CodeCUEEncodeFailed", func(t *testing.T) {
		ctx := context.Background()
		loader := NewLoader(nil)

		value := loader.Context().CompileString(`{name: string}`) // incomplete

		_, err := EncodeYAML(ctx, value)
		if err == nil {
			t.Fatal("expected encode error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEEncodeFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEEncodeFailed, perr.Code())
		}
	})
}
