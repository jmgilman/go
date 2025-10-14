package attributes

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestParseArgsValid(t *testing.T) {
	tests := []struct {
		name     string
		attrText string
		want     map[string]string
	}{
		{
			name:     "single argument",
			attrText: `name="api-server"`,
			want: map[string]string{
				"name": "api-server",
			},
		},
		{
			name:     "multiple arguments",
			attrText: `name="api-server", field="uri"`,
			want: map[string]string{
				"name":  "api-server",
				"field": "uri",
			},
		},
		{
			name:     "arguments with spaces in values",
			attrText: `name="api server", field="container uri"`,
			want: map[string]string{
				"name":  "api server",
				"field": "container uri",
			},
		},
		{
			name:     "arguments with special characters",
			attrText: `name="api-server_v1.0", field="oci://registry.example.com/repo"`,
			want: map[string]string{
				"name":  "api-server_v1.0",
				"field": "oci://registry.example.com/repo",
			},
		},
		{
			name:     "arguments with escaped quotes",
			attrText: `name="value with \"quotes\"", other="test"`,
			want: map[string]string{
				"name":  `value with "quotes"`,
				"other": "test",
			},
		},
		{
			name:     "empty attribute",
			attrText: "",
			want:     map[string]string{},
		},
		{
			name:     "whitespace variations",
			attrText: `name="api-server"  ,  field="uri"`,
			want: map[string]string{
				"name":  "api-server",
				"field": "uri",
			},
		},
		{
			name:     "three arguments",
			attrText: `name="api-server", field="uri", version="v1"`,
			want: map[string]string{
				"name":    "api-server",
				"field":   "uri",
				"version": "v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArgs(tt.attrText)
			if err != nil {
				t.Errorf("ParseArgs() unexpected error = %v", err)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseArgs() got %d args, want %d args", len(got), len(tt.want))
				return
			}
			for key, wantVal := range tt.want {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("ParseArgs() missing key %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("ParseArgs() key %q = %q, want %q", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	tests := []struct {
		name     string
		attrText string
	}{
		{
			name:     "malformed - no equals",
			attrText: `name"api-server"`,
		},
		{
			name:     "malformed - no quotes",
			attrText: `name=api-server`,
		},
		{
			name:     "malformed - missing closing quote",
			attrText: `name="api-server`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseArgs(tt.attrText)
			if err == nil {
				t.Errorf("ParseArgs() expected error, got nil")
			}
		})
	}
}

func TestParseAttribute(t *testing.T) {
	ctx := cuecontext.New()

	tests := []struct {
		name     string
		cueCode  string
		attrName string
		wantArgs map[string]string
		wantOk   bool
	}{
		{
			name:     "attribute exists with single arg",
			cueCode:  `value: "placeholder" @artifact(name="api-server")`,
			attrName: "artifact",
			wantArgs: map[string]string{
				"name": "api-server",
			},
			wantOk: true,
		},
		{
			name:     "attribute exists with multiple args",
			cueCode:  `value: "placeholder" @artifact(name="api-server", field="uri")`,
			attrName: "artifact",
			wantArgs: map[string]string{
				"name":  "api-server",
				"field": "uri",
			},
			wantOk: true,
		},
		{
			name:     "attribute not present",
			cueCode:  `value: "no-attribute"`,
			attrName: "artifact",
			wantArgs: nil,
			wantOk:   false,
		},
		{
			name:     "wrong attribute name",
			cueCode:  `value: "placeholder" @artifact(name="api-server")`,
			attrName: "config",
			wantArgs: nil,
			wantOk:   false,
		},
		{
			name:     "empty attribute no args",
			cueCode:  `value: "placeholder" @artifact()`,
			attrName: "artifact",
			wantArgs: map[string]string{},
			wantOk:   true,
		},
		{
			name:     "attribute with complex values",
			cueCode:  `value: "placeholder" @artifact(name="api-server_v1.0", field="oci://registry/repo")`,
			attrName: "artifact",
			wantArgs: map[string]string{
				"name":  "api-server_v1.0",
				"field": "oci://registry/repo",
			},
			wantOk: true,
		},
		{
			name:     "malformed attribute syntax",
			cueCode:  `value: "placeholder" @artifact(name=no-quotes)`,
			attrName: "artifact",
			wantArgs: nil,
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile the CUE code
			value := ctx.CompileString(tt.cueCode)
			if err := value.Err(); err != nil {
				t.Fatalf("Failed to compile CUE code: %v", err)
			}

			// Get the 'value' field from the compiled code
			fieldValue := value.LookupPath(cue.ParsePath("value"))
			if err := fieldValue.Err(); err != nil {
				t.Fatalf("Failed to lookup 'value' field: %v", err)
			}

			// Parse the attribute
			gotAttr, gotOk := ParseAttribute(fieldValue, tt.attrName)

			if gotOk != tt.wantOk {
				t.Errorf("ParseAttribute() ok = %v, want %v", gotOk, tt.wantOk)
				return
			}

			if !tt.wantOk {
				return
			}

			// Verify attribute name
			if gotAttr.Name != tt.attrName {
				t.Errorf("ParseAttribute() Name = %q, want %q", gotAttr.Name, tt.attrName)
			}

			// Verify arguments
			if len(gotAttr.Args) != len(tt.wantArgs) {
				t.Errorf("ParseAttribute() got %d args, want %d args", len(gotAttr.Args), len(tt.wantArgs))
				return
			}

			for key, wantVal := range tt.wantArgs {
				gotVal, ok := gotAttr.Args[key]
				if !ok {
					t.Errorf("ParseAttribute() missing key %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("ParseAttribute() key %q = %q, want %q", key, gotVal, wantVal)
				}
			}

			// Verify Path is set
			if gotAttr.Path.String() == "" {
				t.Error("ParseAttribute() Path is empty")
			}

			// Verify Value is set and equals the input
			if !gotAttr.Value.Equals(fieldValue) {
				t.Error("ParseAttribute() Value does not equal input value")
			}
		})
	}
}

func TestParseAttributeNestedFields(t *testing.T) {
	ctx := cuecontext.New()

	// Test with nested structure
	cueCode := `
deployment: {
	image: "placeholder" @artifact(name="api-server", field="uri")
	imageDigest: "placeholder" @artifact(name="api-server", field="digest")
}
`
	value := ctx.CompileString(cueCode)
	if err := value.Err(); err != nil {
		t.Fatalf("Failed to compile CUE code: %v", err)
	}

	// Test image field
	imageField := value.LookupPath(cue.ParsePath("deployment.image"))
	if err := imageField.Err(); err != nil {
		t.Fatalf("Failed to lookup deployment.image: %v", err)
	}

	imageAttr, ok := ParseAttribute(imageField, "artifact")
	if !ok {
		t.Fatal("Expected to find artifact attribute on deployment.image")
	}

	if imageAttr.Args["name"] != "api-server" {
		t.Errorf("deployment.image artifact name = %q, want %q", imageAttr.Args["name"], "api-server")
	}
	if imageAttr.Args["field"] != "uri" {
		t.Errorf("deployment.image artifact field = %q, want %q", imageAttr.Args["field"], "uri")
	}

	// Test imageDigest field
	digestField := value.LookupPath(cue.ParsePath("deployment.imageDigest"))
	if err := digestField.Err(); err != nil {
		t.Fatalf("Failed to lookup deployment.imageDigest: %v", err)
	}

	digestAttr, ok := ParseAttribute(digestField, "artifact")
	if !ok {
		t.Fatal("Expected to find artifact attribute on deployment.imageDigest")
	}

	if digestAttr.Args["name"] != "api-server" {
		t.Errorf("deployment.imageDigest artifact name = %q, want %q", digestAttr.Args["name"], "api-server")
	}
	if digestAttr.Args["field"] != "digest" {
		t.Errorf("deployment.imageDigest artifact field = %q, want %q", digestAttr.Args["field"], "digest")
	}
}
