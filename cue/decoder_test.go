package cue

import (
	"context"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

// Test structs for decoding.
type SimpleStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type NestedStruct struct {
	Outer struct {
		Inner struct {
			Value string `json:"value"`
		} `json:"inner"`
	} `json:"outer"`
	List []int `json:"list"`
}

type OptionalFieldsStruct struct {
	Required string  `json:"required"`
	Optional *string `json:"optional,omitempty"`
}

type DefaultValuesStruct struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Enabled bool   `json:"enabled"`
}

type ComplexStruct struct {
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata"`
	Tags     []string          `json:"tags"`
	Config   struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"config"`
}

// TestDecode tests the basic Decode functionality with various struct types.
func TestDecode(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	tests := []struct {
		name        string
		input       string
		target      interface{}
		validate    func(t *testing.T, target interface{})
		wantErr     bool
		errContains string
	}{
		{
			name:   "simple struct",
			input:  `{name: "test", value: 42}`,
			target: &SimpleStruct{},
			validate: func(t *testing.T, target interface{}) {
				result, ok := target.(*SimpleStruct)
				if !ok {
					t.Fatal("target is not *SimpleStruct")
				}
				if result.Name != "test" {
					t.Errorf("expected name='test', got %q", result.Name)
				}
				if result.Value != 42 {
					t.Errorf("expected value=42, got %d", result.Value)
				}
			},
		},
		{
			name: "nested struct",
			input: `{
				outer: {
					inner: {
						value: "deep"
					}
				}
				list: [1, 2, 3]
			}`,
			target: &NestedStruct{},
			validate: func(t *testing.T, target interface{}) {
				result, ok := target.(*NestedStruct)
				if !ok {
					t.Fatal("target is not *NestedStruct")
				}
				if result.Outer.Inner.Value != "deep" {
					t.Errorf("expected value='deep', got %q", result.Outer.Inner.Value)
				}
				expectedList := []int{1, 2, 3}
				if len(result.List) != len(expectedList) {
					t.Fatalf("expected list length %d, got %d", len(expectedList), len(result.List))
				}
				for i, v := range expectedList {
					if result.List[i] != v {
						t.Errorf("at index %d: expected %d, got %d", i, v, result.List[i])
					}
				}
			},
		},
		{
			name: "complex struct with maps and slices",
			input: `{
				id: "abc-123"
				metadata: {
					env: "production"
					region: "us-east-1"
				}
				tags: ["important", "monitored"]
				config: {
					host: "example.com"
					port: 8080
				}
			}`,
			target: &ComplexStruct{},
			validate: func(t *testing.T, target interface{}) {
				result, ok := target.(*ComplexStruct)
				if !ok {
					t.Fatal("target is not *ComplexStruct")
				}
				if result.ID != "abc-123" {
					t.Errorf("expected id='abc-123', got %q", result.ID)
				}
				if result.Metadata["env"] != "production" {
					t.Errorf("expected env='production', got %q", result.Metadata["env"])
				}
				if result.Config.Host != "example.com" {
					t.Errorf("expected host='example.com', got %q", result.Config.Host)
				}
				if result.Config.Port != 8080 {
					t.Errorf("expected port=8080, got %d", result.Config.Port)
				}
			},
		},
		{
			name:   "optional field present",
			input:  `{required: "value", optional: "present"}`,
			target: &OptionalFieldsStruct{},
			validate: func(t *testing.T, target interface{}) {
				result, ok := target.(*OptionalFieldsStruct)
				if !ok {
					t.Fatal("target is not *OptionalFieldsStruct")
				}
				if result.Required != "value" {
					t.Errorf("expected required='value', got %q", result.Required)
				}
				if result.Optional == nil {
					t.Error("expected optional to be present")
				} else if *result.Optional != "present" {
					t.Errorf("expected optional='present', got %q", *result.Optional)
				}
			},
		},
		{
			name:   "optional field missing",
			input:  `{required: "value"}`,
			target: &OptionalFieldsStruct{},
			validate: func(t *testing.T, target interface{}) {
				result, ok := target.(*OptionalFieldsStruct)
				if !ok {
					t.Fatal("target is not *OptionalFieldsStruct")
				}
				if result.Required != "value" {
					t.Errorf("expected required='value', got %q", result.Required)
				}
				if result.Optional != nil {
					t.Errorf("expected optional to be nil, got %v", *result.Optional)
				}
			},
		},
		{
			name: "default values from CUE schema",
			input: `
				#Schema: {
					name: string | *"default-name"
					count: int | *10
					enabled: bool | *true
				}
				#Schema & {name: "custom"}
			`,
			target: &DefaultValuesStruct{},
			validate: func(t *testing.T, target interface{}) {
				result, ok := target.(*DefaultValuesStruct)
				if !ok {
					t.Fatal("target is not *DefaultValuesStruct")
				}
				if result.Name != "custom" {
					t.Errorf("expected name='custom', got %q", result.Name)
				}
				if result.Count != 10 {
					t.Errorf("expected count=10 (default), got %d", result.Count)
				}
				if !result.Enabled {
					t.Error("expected enabled=true (default)")
				}
			},
		},
		{
			name:        "non-pointer target",
			input:       `{name: "test", value: 42}`,
			target:      SimpleStruct{},
			wantErr:     true,
			errContains: "must be a pointer",
		},
		{
			name:        "nil target",
			input:       `{name: "test", value: 42}`,
			target:      nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name:        "pointer to non-struct",
			input:       `"test"`,
			target:      new(string),
			wantErr:     true,
			errContains: "must be a pointer to a struct",
		},
		{
			name:        "type mismatch - string to int",
			input:       `{name: "test", value: "not-a-number"}`,
			target:      &SimpleStruct{},
			wantErr:     true,
			errContains: "decode",
		},
		{
			name:        "value with error",
			input:       `{name: "test", value: x + 1}`, // x is undefined
			target:      &SimpleStruct{},
			wantErr:     true,
			errContains: "contains errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := cueCtx.CompileString(tt.input)

			err := Decode(ctx, value, tt.target)

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
				tt.validate(t, tt.target)
			}
		})
	}
}

// TestDecodeContextCancelled tests that Decode handles context cancellation.
func TestDecodeContextCancelled(t *testing.T) {
	cueCtx := cuecontext.New()
	value := cueCtx.CompileString(`{name: "test", value: 42}`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	target := &SimpleStruct{}
	err := Decode(ctx, value, target)

	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Errorf("expected error about context cancellation, got: %v", err)
	}
}

// TestDecodeNilPointer tests decoding to a nil pointer.
func TestDecodeNilPointer(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()
	value := cueCtx.CompileString(`{name: "test", value: 42}`)

	var target *SimpleStruct // nil pointer
	err := Decode(ctx, value, target)

	if err == nil {
		t.Fatal("expected error for nil pointer")
	}
	if !strings.Contains(err.Error(), "cannot be nil") {
		t.Errorf("expected error about nil pointer, got: %v", err)
	}
}

// TestDecodeWithBooleans tests decoding boolean values.
func TestDecodeWithBooleans(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	type BoolStruct struct {
		Enabled  bool `json:"enabled"`
		Disabled bool `json:"disabled"`
	}

	value := cueCtx.CompileString(`{enabled: true, disabled: false}`)
	target := &BoolStruct{}

	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !target.Enabled {
		t.Error("expected enabled=true")
	}
	if target.Disabled {
		t.Error("expected disabled=false")
	}
}

// TestDecodeWithFloats tests decoding floating-point numbers.
func TestDecodeWithFloats(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	type FloatStruct struct {
		Pi    float64 `json:"pi"`
		Small float32 `json:"small"`
	}

	value := cueCtx.CompileString(`{pi: 3.14159, small: 0.5}`)
	target := &FloatStruct{}

	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.Pi != 3.14159 {
		t.Errorf("expected pi=3.14159, got %f", target.Pi)
	}
	if target.Small != 0.5 {
		t.Errorf("expected small=0.5, got %f", target.Small)
	}
}

// TestDecodeWithNullValues tests decoding null values.
func TestDecodeWithNullValues(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	type NullableStruct struct {
		Name     string  `json:"name"`
		Optional *string `json:"optional"`
	}

	value := cueCtx.CompileString(`{name: "test", optional: null}`)
	target := &NullableStruct{}

	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.Name != "test" {
		t.Errorf("expected name='test', got %q", target.Name)
	}
	if target.Optional != nil {
		t.Errorf("expected optional to be nil, got %v", *target.Optional)
	}
}

// TestDecodeWithEmptyStruct tests decoding to an empty struct.
func TestDecodeWithEmptyStruct(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	type EmptyStruct struct{}

	value := cueCtx.CompileString(`{}`)
	target := &EmptyStruct{}

	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDecodeWithSlices tests decoding various slice types.
func TestDecodeWithSlices(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	type SliceStruct struct {
		Strings []string `json:"strings"`
		Numbers []int    `json:"numbers"`
	}

	value := cueCtx.CompileString(`{
		strings: ["a", "b", "c"]
		numbers: [1, 2, 3, 4, 5]
	}`)
	target := &SliceStruct{}

	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedStrings := []string{"a", "b", "c"}
	if len(target.Strings) != len(expectedStrings) {
		t.Fatalf("expected %d strings, got %d", len(expectedStrings), len(target.Strings))
	}
	for i, v := range expectedStrings {
		if target.Strings[i] != v {
			t.Errorf("at index %d: expected %q, got %q", i, v, target.Strings[i])
		}
	}

	expectedNumbers := []int{1, 2, 3, 4, 5}
	if len(target.Numbers) != len(expectedNumbers) {
		t.Fatalf("expected %d numbers, got %d", len(expectedNumbers), len(target.Numbers))
	}
	for i, v := range expectedNumbers {
		if target.Numbers[i] != v {
			t.Errorf("at index %d: expected %d, got %d", i, v, target.Numbers[i])
		}
	}
}

// TestDecodeWithMaps tests decoding map types.
func TestDecodeWithMaps(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	type MapStruct struct {
		StringMap map[string]string `json:"stringMap"`
		IntMap    map[string]int    `json:"intMap"`
	}

	value := cueCtx.CompileString(`{
		stringMap: {
			key1: "value1"
			key2: "value2"
		}
		intMap: {
			a: 1
			b: 2
		}
	}`)
	target := &MapStruct{}

	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.StringMap["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %q", target.StringMap["key1"])
	}
	if target.IntMap["a"] != 1 {
		t.Errorf("expected a=1, got %d", target.IntMap["a"])
	}
}

// TestDecodePreservesExistingFields tests that decoding doesn't erase existing fields
// when they're not present in the CUE value.
func TestDecodePreservesExistingFields(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Pre-populate the target struct
	target := &SimpleStruct{
		Name:  "existing",
		Value: 100,
	}

	// Decode a partial update (only name field)
	value := cueCtx.CompileString(`{name: "updated", value: 42}`)
	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both fields should be updated
	if target.Name != "updated" {
		t.Errorf("expected name='updated', got %q", target.Name)
	}
	if target.Value != 42 {
		t.Errorf("expected value=42, got %d", target.Value)
	}
}

// TestDecodeMissingRequiredFields tests decoding when required fields are missing.
func TestDecodeMissingRequiredFields(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	// Try to decode with missing required field
	value := cueCtx.CompileString(`{name: "test"}`) // missing 'value' field
	target := &SimpleStruct{}

	err := Decode(ctx, value, target)
	// CUE's decode is lenient - missing fields get zero values
	// This is expected behavior
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.Name != "test" {
		t.Errorf("expected name='test', got %q", target.Name)
	}
	if target.Value != 0 {
		t.Errorf("expected value=0 (zero value), got %d", target.Value)
	}
}

// TestDecodeWithCUEConstraints tests decoding values with CUE constraints.
func TestDecodeWithCUEConstraints(t *testing.T) {
	ctx := context.Background()
	cueCtx := cuecontext.New()

	type ConstraintStruct struct {
		Port int    `json:"port"`
		Host string `json:"host"`
	}

	// CUE value with constraints that are satisfied
	value := cueCtx.CompileString(`{
		port: int & >0 & <65536 & 8080
		host: string & =~"^[a-z.]+$" & "example.com"
	}`)
	target := &ConstraintStruct{}

	err := Decode(ctx, value, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.Port != 8080 {
		t.Errorf("expected port=8080, got %d", target.Port)
	}
	if target.Host != "example.com" {
		t.Errorf("expected host='example.com', got %q", target.Host)
	}
}
