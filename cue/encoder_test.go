package cue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"gopkg.in/yaml.v3"
)

// TestEncodeYAML tests the EncodeYAML function with various inputs.
func TestEncodeYAML(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, output []byte)
	}{
		{
			name:  "simple string value",
			input: `"hello world"`,
			validate: func(t *testing.T, output []byte) {
				var result string
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				if result != "hello world" {
					t.Errorf("expected 'hello world', got %q", result)
				}
			},
		},
		{
			name:  "simple number",
			input: `42`,
			validate: func(t *testing.T, output []byte) {
				var result int
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				if result != 42 {
					t.Errorf("expected 42, got %d", result)
				}
			},
		},
		{
			name:  "simple struct",
			input: `{name: "test", value: 123}`,
			validate: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				if result["name"] != "test" {
					t.Errorf("expected name='test', got %v", result["name"])
				}
				if result["value"] != 123 {
					t.Errorf("expected value=123, got %v", result["value"])
				}
			},
		},
		{
			name: "nested structure",
			input: `{
				outer: {
					inner: {
						value: "deep"
					}
				}
				list: [1, 2, 3]
			}`,
			validate: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				outer, ok := result["outer"].(map[string]interface{})
				if !ok {
					t.Fatal("outer is not a map")
				}
				inner, ok := outer["inner"].(map[string]interface{})
				if !ok {
					t.Fatal("inner is not a map")
				}
				if inner["value"] != "deep" {
					t.Errorf("expected value='deep', got %v", inner["value"])
				}
			},
		},
		{
			name:  "list of values",
			input: `[1, 2, 3, 4, 5]`,
			validate: func(t *testing.T, output []byte) {
				var result []int
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				expected := []int{1, 2, 3, 4, 5}
				if len(result) != len(expected) {
					t.Fatalf("expected %d elements, got %d", len(expected), len(result))
				}
				for i, v := range expected {
					if result[i] != v {
						t.Errorf("at index %d: expected %d, got %d", i, v, result[i])
					}
				}
			},
		},
		{
			name:        "incomplete value (not concrete)",
			input:       `{name: string, value: int}`, // Schema, not concrete value
			wantErr:     true,
			errContains: "encode", // Error from encoder
		},
		{
			name:        "value with error",
			input:       `{x: 1, y: x + z}`, // z is undefined
			wantErr:     true,
			errContains: "contains errors",
		},
		{
			name:  "boolean values",
			input: `{enabled: true, disabled: false}`,
			validate: func(t *testing.T, output []byte) {
				var result map[string]bool
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				if !result["enabled"] {
					t.Error("expected enabled=true")
				}
				if result["disabled"] {
					t.Error("expected disabled=false")
				}
			},
		},
		{
			name:  "null value",
			input: `null`,
			validate: func(t *testing.T, output []byte) {
				// YAML representation of null should be "null\n"
				if string(bytes.TrimSpace(output)) != "null" {
					t.Errorf("expected 'null', got %q", string(output))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := cueCtx.CompileString(tt.input)

			result, err := EncodeYAML(ctx, value)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// TestEncodeYAMLContextCancelled tests that EncodeYAML handles context cancellation.
func TestEncodeYAMLContextCancelled(t *testing.T) {
	cueCtx := cuecontext.New()
	value := cueCtx.CompileString(`"test"`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := EncodeYAML(ctx, value)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Errorf("expected error about context cancellation, got: %v", err)
	}
}

// TestEncodeJSON tests the EncodeJSON function with various inputs.
func TestEncodeJSON(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, output []byte)
	}{
		{
			name:  "simple string value",
			input: `"hello world"`,
			validate: func(t *testing.T, output []byte) {
				var result string
				if err := json.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result != "hello world" {
					t.Errorf("expected 'hello world', got %q", result)
				}
			},
		},
		{
			name:  "simple number",
			input: `42`,
			validate: func(t *testing.T, output []byte) {
				var result int
				if err := json.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result != 42 {
					t.Errorf("expected 42, got %d", result)
				}
			},
		},
		{
			name:  "simple object",
			input: `{name: "test", value: 123}`,
			validate: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result["name"] != "test" {
					t.Errorf("expected name='test', got %v", result["name"])
				}
				// JSON numbers are float64 by default
				if result["value"] != float64(123) {
					t.Errorf("expected value=123, got %v", result["value"])
				}
			},
		},
		{
			name: "nested structure",
			input: `{
				outer: {
					inner: {
						value: "deep"
					}
				}
				list: [1, 2, 3]
			}`,
			validate: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				outer, ok := result["outer"].(map[string]interface{})
				if !ok {
					t.Fatal("outer is not a map")
				}
				inner, ok := outer["inner"].(map[string]interface{})
				if !ok {
					t.Fatal("inner is not a map")
				}
				if inner["value"] != "deep" {
					t.Errorf("expected value='deep', got %v", inner["value"])
				}
			},
		},
		{
			name:  "array of values",
			input: `[1, 2, 3, 4, 5]`,
			validate: func(t *testing.T, output []byte) {
				var result []interface{}
				if err := json.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				expected := []float64{1, 2, 3, 4, 5}
				if len(result) != len(expected) {
					t.Fatalf("expected %d elements, got %d", len(expected), len(result))
				}
				for i, v := range expected {
					if result[i] != v {
						t.Errorf("at index %d: expected %v, got %v", i, v, result[i])
					}
				}
			},
		},
		{
			name:        "incomplete value (not concrete)",
			input:       `{name: string, value: int}`, // Schema, not concrete value
			wantErr:     true,
			errContains: "encode", // Error from encoder
		},
		{
			name:        "value with error",
			input:       `{x: 1, y: x + z}`, // z is undefined
			wantErr:     true,
			errContains: "contains errors",
		},
		{
			name:  "boolean values",
			input: `{enabled: true, disabled: false}`,
			validate: func(t *testing.T, output []byte) {
				var result map[string]bool
				if err := json.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if !result["enabled"] {
					t.Error("expected enabled=true")
				}
				if result["disabled"] {
					t.Error("expected disabled=false")
				}
			},
		},
		{
			name:  "null value",
			input: `null`,
			validate: func(t *testing.T, output []byte) {
				if string(bytes.TrimSpace(output)) != "null" {
					t.Errorf("expected 'null', got %q", string(output))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := cueCtx.CompileString(tt.input)

			result, err := EncodeJSON(ctx, value)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// TestEncodeJSONContextCancelled tests that EncodeJSON handles context cancellation.
func TestEncodeJSONContextCancelled(t *testing.T) {
	cueCtx := cuecontext.New()
	value := cueCtx.CompileString(`"test"`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := EncodeJSON(ctx, value)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Errorf("expected error about context cancellation, got: %v", err)
	}
}

// TestEncodeYAMLStream tests the streaming YAML encoder.
func TestEncodeYAMLStream(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	tests := []struct {
		name        string
		input       string
		writer      io.Writer
		wantErr     bool
		errContains string
		validate    func(t *testing.T, output []byte)
	}{
		{
			name:   "simple value to buffer",
			input:  `{name: "test", value: 123}`,
			writer: &bytes.Buffer{},
			validate: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				if result["name"] != "test" {
					t.Errorf("expected name='test', got %v", result["name"])
				}
			},
		},
		{
			name: "large nested structure",
			input: `{
				level1: {
					level2: {
						level3: {
							data: [1, 2, 3, 4, 5]
						}
					}
				}
				metadata: {
					version: "1.0"
					tags: ["a", "b", "c"]
				}
			}`,
			writer: &bytes.Buffer{},
			validate: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				if err := yaml.Unmarshal(output, &result); err != nil {
					t.Fatalf("failed to unmarshal YAML: %v", err)
				}
				// Verify structure is present
				if result["level1"] == nil {
					t.Error("level1 missing")
				}
				if result["metadata"] == nil {
					t.Error("metadata missing")
				}
			},
		},
		{
			name:        "nil writer",
			input:       `"test"`,
			writer:      nil,
			wantErr:     true,
			errContains: "writer cannot be nil",
		},
		{
			name:        "incomplete value",
			input:       `{name: string}`,
			writer:      &bytes.Buffer{},
			wantErr:     true,
			errContains: "encode", // Error comes from encoder
		},
		{
			name:        "value with error",
			input:       `{x: 1, y: x + z}`,
			writer:      &bytes.Buffer{},
			wantErr:     true,
			errContains: "contains errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := cueCtx.CompileString(tt.input)

			err := EncodeYAMLStream(ctx, value, tt.writer)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil && tt.writer != nil {
				buf, ok := tt.writer.(*bytes.Buffer)
				if !ok {
					t.Fatal("writer is not a buffer")
				}
				tt.validate(t, buf.Bytes())
			}
		})
	}
}

// TestEncodeYAMLStreamContextCancelled tests context cancellation.
func TestEncodeYAMLStreamContextCancelled(t *testing.T) {
	cueCtx := cuecontext.New()
	value := cueCtx.CompileString(`"test"`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	buf := &bytes.Buffer{}
	err := EncodeYAMLStream(ctx, value, buf)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Errorf("expected error about context cancellation, got: %v", err)
	}
}

// failingWriter always returns an error on Write.
type failingWriter struct {
	failAfter int
	written   int
}

func (w *failingWriter) Write(p []byte) (n int, err error) {
	if w.written >= w.failAfter {
		return 0, errors.New("write failed")
	}
	w.written += len(p)
	return len(p), nil
}

// TestEncodeYAMLStreamWriteError tests handling of writer errors.
func TestEncodeYAMLStreamWriteError(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	value := cueCtx.CompileString(`{name: "test", value: 123}`)

	writer := &failingWriter{failAfter: 0}
	err := EncodeYAMLStream(ctx, value, writer)

	if err == nil {
		t.Fatal("expected error from failing writer")
	}
	if !strings.Contains(err.Error(), "failed to write") {
		t.Errorf("expected error about write failure, got: %v", err)
	}
}

// partialWriter only writes a portion of the data.
type partialWriter struct {
	written int
}

func (w *partialWriter) Write(p []byte) (n int, err error) {
	// Write only half of the bytes
	n = len(p) / 2
	if n == 0 && len(p) > 0 {
		n = 1 // Write at least one byte if there's data
	}
	w.written += n
	return n, nil
}

// TestEncodeYAMLStreamPartialWrite tests handling of incomplete writes.
func TestEncodeYAMLStreamPartialWrite(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	// Create a larger value to ensure the partial write happens
	value := cueCtx.CompileString(`{
		name: "test"
		value: 123
		nested: {
			a: "value"
			b: "another"
			c: "more data"
		}
	}`)

	writer := &partialWriter{}
	err := EncodeYAMLStream(ctx, value, writer)

	if err == nil {
		t.Fatal("expected error from partial write")
	}
	if !strings.Contains(err.Error(), "incomplete write") {
		t.Errorf("expected error about incomplete write, got: %v", err)
	}
	// Verify some bytes were written
	if writer.written == 0 {
		t.Error("expected some bytes to be written")
	}
}

// TestEncodeYAMLvsJSON tests that YAML and JSON produce equivalent data.
func TestEncodeYAMLvsJSON(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	input := `{
		name: "test"
		value: 123
		nested: {
			inner: "data"
		}
		list: [1, 2, 3]
	}`

	value := cueCtx.CompileString(input)

	yamlBytes, err := EncodeYAML(ctx, value)
	if err != nil {
		t.Fatalf("EncodeYAML failed: %v", err)
	}

	jsonBytes, err := EncodeJSON(ctx, value)
	if err != nil {
		t.Fatalf("EncodeJSON failed: %v", err)
	}

	// Parse both and compare
	var yamlResult, jsonResult map[string]interface{}

	if err := yaml.Unmarshal(yamlBytes, &yamlResult); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if err := json.Unmarshal(jsonBytes, &jsonResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Compare key fields (deep comparison would be complex due to type differences)
	if yamlResult["name"] != jsonResult["name"] {
		t.Errorf("name mismatch: YAML=%v, JSON=%v", yamlResult["name"], jsonResult["name"])
	}

	// Note: JSON unmarshals numbers as float64, YAML as int, so we need type assertion
	yamlValue, _ := yamlResult["value"].(int)
	jsonValue, _ := jsonResult["value"].(float64)
	if float64(yamlValue) != jsonValue {
		t.Errorf("value mismatch: YAML=%v, JSON=%v", yamlValue, jsonValue)
	}
}
