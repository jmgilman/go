package cue

import (
	"context"
	"errors"
	"testing"

	"cuelang.org/go/cue"
	platformerrors "github.com/jmgilman/go/errors"
	"github.com/jmgilman/go/fs/billy"
)

// TestLoadModule tests loading a CUE module from a filesystem.
func TestLoadModule(t *testing.T) {
	t.Run("loads valid module with single file", func(t *testing.T) {
		// Setup in-memory filesystem
		mfs := billy.NewMemory()
		if err := mfs.WriteFile("test.cue", []byte(`
			package test
			value: "hello"
		`), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		// Load module from root directory
		result, err := loader.LoadModule(ctx, ".")
		if err != nil {
			t.Fatalf("LoadModule failed: %v", err)
		}

		// Verify the loaded value
		if result.Err() != nil {
			t.Fatalf("loaded value has error: %v", result.Err())
		}

		// Check that we can lookup the value
		valueField := result.LookupPath(cue.ParsePath("value"))
		if valueField.Err() != nil {
			t.Fatalf("failed to lookup value field: %v", valueField.Err())
		}

		str, err := valueField.String()
		if err != nil {
			t.Fatalf("failed to extract string: %v", err)
		}

		if str != "hello" {
			t.Errorf("expected value='hello', got value=%q", str)
		}
	})

	t.Run("loads valid module with multiple files", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create multiple CUE files
		if err := mfs.WriteFile("file1.cue", []byte(`
			package test
			field1: "value1"
		`), 0644); err != nil {
			t.Fatalf("failed to create file1.cue: %v", err)
		}

		if err := mfs.WriteFile("file2.cue", []byte(`
			package test
			field2: "value2"
		`), 0644); err != nil {
			t.Fatalf("failed to create file2.cue: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		result, err := loader.LoadModule(ctx, ".")
		if err != nil {
			t.Fatalf("LoadModule failed: %v", err)
		}

		// Verify both fields are present
		field1 := result.LookupPath(cue.ParsePath("field1"))
		if field1.Err() != nil {
			t.Fatalf("field1 not found: %v", field1.Err())
		}

		field2 := result.LookupPath(cue.ParsePath("field2"))
		if field2.Err() != nil {
			t.Fatalf("field2 not found: %v", field2.Err())
		}
	})

	t.Run("loads module with cross-package imports", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create cue.mod/module.cue
		if err := mfs.MkdirAll("cue.mod", 0755); err != nil {
			t.Fatalf("failed to create cue.mod directory: %v", err)
		}
		if err := mfs.WriteFile("cue.mod/module.cue", []byte(`
module: "example.com/mymodule"
language: {
	version: "v0.14.0"
}
`), 0644); err != nil {
			t.Fatalf("failed to create cue.mod/module.cue: %v", err)
		}

		// Create a package with common types
		if err := mfs.MkdirAll("common", 0755); err != nil {
			t.Fatalf("failed to create common directory: %v", err)
		}
		if err := mfs.WriteFile("common/types.cue", []byte(`
package common

#Person: {
	name: string
	age: int
}
`), 0644); err != nil {
			t.Fatalf("failed to create common/types.cue: %v", err)
		}

		// Create main package that imports common
		if err := mfs.WriteFile("main.cue", []byte(`
package main

import "example.com/mymodule/common"

person: common.#Person & {
	name: "Alice"
	age: 30
}
`), 0644); err != nil {
			t.Fatalf("failed to create main.cue: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		// Load the module - this should now work with cross-package imports
		result, err := loader.LoadModule(ctx, ".")
		if err != nil {
			t.Fatalf("LoadModule failed with cross-package imports: %v", err)
		}

		// Verify the loaded value includes the imported type
		personField := result.LookupPath(cue.ParsePath("person"))
		if personField.Err() != nil {
			t.Fatalf("failed to lookup person field: %v", personField.Err())
		}

		// Verify the person has the correct structure
		nameField := personField.LookupPath(cue.ParsePath("name"))
		if nameField.Err() != nil {
			t.Fatalf("failed to lookup name field: %v", nameField.Err())
		}

		name, err := nameField.String()
		if err != nil {
			t.Fatalf("failed to extract name: %v", err)
		}

		if name != "Alice" {
			t.Errorf("expected name='Alice', got name=%q", name)
		}

		ageField := personField.LookupPath(cue.ParsePath("age"))
		if ageField.Err() != nil {
			t.Fatalf("failed to lookup age field: %v", ageField.Err())
		}

		age, err := ageField.Int64()
		if err != nil {
			t.Fatalf("failed to extract age: %v", err)
		}

		if age != 30 {
			t.Errorf("expected age=30, got age=%d", age)
		}
	})

	t.Run("returns error when module directory does not exist", func(t *testing.T) {
		mfs := billy.NewMemory()
		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadModule(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent directory, got nil")
		}

		// Check error code
		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}

		// Check context contains module_path
		errCtx := perr.Context()
		if errCtx == nil {
			t.Fatal("expected error context, got nil")
		}

		if modulePath, ok := errCtx["module_path"]; !ok || modulePath != "nonexistent" {
			t.Errorf("expected module_path='nonexistent' in context, got %v", modulePath)
		}
	})

	t.Run("returns error when module contains no CUE files", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create non-CUE file
		if err := mfs.WriteFile("readme.txt", []byte("not a cue file"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadModule(ctx, ".")
		if err == nil {
			t.Fatal("expected error for module with no CUE files, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}
	})

	t.Run("returns error on invalid CUE syntax", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create invalid CUE file
		if err := mfs.WriteFile("invalid.cue", []byte(`
			package test
			this is not valid CUE syntax
		`), 0644); err != nil {
			t.Fatalf("failed to create invalid.cue: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadModule(ctx, ".")
		if err == nil {
			t.Fatal("expected error for invalid CUE syntax, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEBuildFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEBuildFailed, perr.Code())
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		mfs := billy.NewMemory()
		if err := mfs.WriteFile("test.cue", []byte(`package test`), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		loader := NewLoader(mfs)

		_, err := loader.LoadModule(ctx, ".")
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}
	})
}

// TestLoadFile tests loading a single CUE file.
func TestLoadFile(t *testing.T) {
	t.Run("loads single file successfully", func(t *testing.T) {
		mfs := billy.NewMemory()
		if err := mfs.WriteFile("test.cue", []byte(`
			package test
			value: 42
		`), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		result, err := loader.LoadFile(ctx, "test.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		// Verify the loaded value
		valueField := result.LookupPath(cue.ParsePath("value"))
		if valueField.Err() != nil {
			t.Fatalf("failed to lookup value field: %v", valueField.Err())
		}

		num, err := valueField.Int64()
		if err != nil {
			t.Fatalf("failed to extract int: %v", err)
		}

		if num != 42 {
			t.Errorf("expected value=42, got value=%d", num)
		}
	})

	t.Run("returns error when file does not exist", func(t *testing.T) {
		mfs := billy.NewMemory()
		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadFile(ctx, "nonexistent.cue")
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		mfs := billy.NewMemory()
		if err := mfs.WriteFile("test.cue", []byte(`package test`), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		loader := NewLoader(mfs)

		_, err := loader.LoadFile(ctx, "test.cue")
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}
	})
}

// TestLoadPackage tests loading a package (multiple files in a directory).
func TestLoadPackage(t *testing.T) {
	t.Run("unifies multiple files correctly", func(t *testing.T) {
		mfs := billy.NewMemory()

		if err := mfs.WriteFile("file1.cue", []byte(`
			package test
			value: string
		`), 0644); err != nil {
			t.Fatalf("failed to create file1.cue: %v", err)
		}

		if err := mfs.WriteFile("file2.cue", []byte(`
			package test
			value: "hello"
		`), 0644); err != nil {
			t.Fatalf("failed to create file2.cue: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		result, err := loader.LoadPackage(ctx, ".")
		if err != nil {
			t.Fatalf("LoadPackage failed: %v", err)
		}

		// Verify the unified value
		valueField := result.LookupPath(cue.ParsePath("value"))
		if valueField.Err() != nil {
			t.Fatalf("failed to lookup value field: %v", valueField.Err())
		}

		str, err := valueField.String()
		if err != nil {
			t.Fatalf("failed to extract string: %v", err)
		}

		if str != "hello" {
			t.Errorf("expected value='hello', got value=%q", str)
		}
	})

	t.Run("returns error when package directory does not exist", func(t *testing.T) {
		mfs := billy.NewMemory()
		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadPackage(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent directory, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}
	})

	t.Run("returns error on unification conflict", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create conflicting files
		if err := mfs.WriteFile("file1.cue", []byte(`
			package test
			value: 1
		`), 0644); err != nil {
			t.Fatalf("failed to create file1.cue: %v", err)
		}

		if err := mfs.WriteFile("file2.cue", []byte(`
			package test
			value: 2
		`), 0644); err != nil {
			t.Fatalf("failed to create file2.cue: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadPackage(ctx, ".")
		if err == nil {
			t.Fatal("expected error for unification conflict, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEBuildFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEBuildFailed, perr.Code())
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		mfs := billy.NewMemory()
		if err := mfs.WriteFile("test.cue", []byte(`package test`), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		loader := NewLoader(mfs)

		_, err := loader.LoadPackage(ctx, ".")
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUELoadFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUELoadFailed, perr.Code())
		}
	})
}

// TestLoadBytes tests loading CUE from byte slices.
func TestLoadBytes(t *testing.T) {
	t.Run("loads valid CUE source", func(t *testing.T) {
		source := []byte(`
			package test
			name: "example"
			count: 10
		`)

		ctx := context.Background()
		loader := NewLoader(nil) // No filesystem needed for LoadBytes

		result, err := loader.LoadBytes(ctx, source, "test.cue")
		if err != nil {
			t.Fatalf("LoadBytes failed: %v", err)
		}

		// Verify loaded values
		nameField := result.LookupPath(cue.ParsePath("name"))
		if nameField.Err() != nil {
			t.Fatalf("failed to lookup name field: %v", nameField.Err())
		}

		name, err := nameField.String()
		if err != nil {
			t.Fatalf("failed to extract name string: %v", err)
		}

		if name != "example" {
			t.Errorf("expected name='example', got name=%q", name)
		}
	})

	t.Run("uses synthetic filename when empty", func(t *testing.T) {
		source := []byte(`package test`)

		ctx := context.Background()
		loader := NewLoader(nil)

		_, err := loader.LoadBytes(ctx, source, "")
		if err != nil {
			t.Fatalf("LoadBytes failed: %v", err)
		}
	})

	t.Run("returns error on invalid CUE syntax", func(t *testing.T) {
		source := []byte(`
			package test
			this is invalid
		`)

		ctx := context.Background()
		loader := NewLoader(nil)

		_, err := loader.LoadBytes(ctx, source, "invalid.cue")
		if err == nil {
			t.Fatal("expected error for invalid CUE syntax, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEBuildFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEBuildFailed, perr.Code())
		}

		// Check context contains filename
		errCtx := perr.Context()
		if errCtx == nil {
			t.Fatal("expected error context, got nil")
		}

		if filename, ok := errCtx["filename"]; !ok || filename != "invalid.cue" {
			t.Errorf("expected filename='invalid.cue' in context, got %v", filename)
		}
	})

	t.Run("returns error on incomplete CUE", func(t *testing.T) {
		source := []byte(`
			package test
			value: string
		`)

		ctx := context.Background()
		loader := NewLoader(nil)

		// This should succeed - incomplete values are valid in CUE
		result, err := loader.LoadBytes(ctx, source, "incomplete.cue")
		if err != nil {
			t.Fatalf("LoadBytes failed: %v", err)
		}

		// But accessing the incomplete field should show it's not concrete
		valueField := result.LookupPath(cue.ParsePath("value"))
		if valueField.Err() != nil {
			t.Fatalf("failed to lookup value field: %v", valueField.Err())
		}

		if valueField.IsConcrete() {
			t.Error("expected value to be incomplete, but it's concrete")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		source := []byte(`package test`)

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		loader := NewLoader(nil)

		_, err := loader.LoadBytes(ctx, source, "test.cue")
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		if perr.Code() != platformerrors.CodeCUEBuildFailed {
			t.Errorf("expected code %s, got %s", platformerrors.CodeCUEBuildFailed, perr.Code())
		}
	})
}

// TestHelperFunctions tests internal helper functions.
func TestHelperFunctions(t *testing.T) {
	t.Run("discoverCueFiles finds .cue files", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create directory structure with CUE files
		if err := mfs.MkdirAll("subdir", 0755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}

		if err := mfs.WriteFile("root.cue", []byte(`package test`), 0644); err != nil {
			t.Fatalf("failed to create root.cue: %v", err)
		}

		if err := mfs.WriteFile("subdir/sub.cue", []byte(`package test`), 0644); err != nil {
			t.Fatalf("failed to create subdir/sub.cue: %v", err)
		}

		if err := mfs.WriteFile("readme.txt", []byte(`not cue`), 0644); err != nil {
			t.Fatalf("failed to create readme.txt: %v", err)
		}

		files, err := discoverCueFiles(mfs, ".")
		if err != nil {
			t.Fatalf("discoverCueFiles failed: %v", err)
		}

		if len(files) != 2 {
			t.Errorf("expected 2 CUE files, got %d", len(files))
		}
	})

	t.Run("fileExists correctly detects files", func(t *testing.T) {
		mfs := billy.NewMemory()

		if err := mfs.WriteFile("exists.txt", []byte(`content`), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		exists, err := fileExists(mfs, "exists.txt")
		if err != nil {
			t.Fatalf("fileExists failed: %v", err)
		}
		if !exists {
			t.Error("expected file to exist")
		}

		exists, err = fileExists(mfs, "nonexistent.txt")
		if err != nil {
			t.Fatalf("fileExists failed for nonexistent file: %v", err)
		}
		if exists {
			t.Error("expected file to not exist")
		}
	})
}

// TestErrorContextMetadata verifies error context includes expected metadata.
func TestErrorContextMetadata(t *testing.T) {
	t.Run("LoadModule error includes module_path", func(t *testing.T) {
		mfs := billy.NewMemory()
		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadModule(ctx, "test/module")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		errCtx := perr.Context()
		if errCtx == nil {
			t.Fatal("expected error context, got nil")
		}

		if modulePath, ok := errCtx["module_path"]; !ok || modulePath != "test/module" {
			t.Errorf("expected module_path='test/module' in context, got %v", modulePath)
		}
	})

	t.Run("LoadFile error includes file_path", func(t *testing.T) {
		mfs := billy.NewMemory()
		ctx := context.Background()
		loader := NewLoader(mfs)

		_, err := loader.LoadFile(ctx, "missing.cue")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		errCtx := perr.Context()
		if errCtx == nil {
			t.Fatal("expected error context, got nil")
		}

		if filePath, ok := errCtx["file_path"]; !ok || filePath != "missing.cue" {
			t.Errorf("expected file_path='missing.cue' in context, got %v", filePath)
		}
	})

	t.Run("LoadBytes error includes filename", func(t *testing.T) {
		source := []byte(`invalid syntax`)

		ctx := context.Background()
		loader := NewLoader(nil)

		_, err := loader.LoadBytes(ctx, source, "test.cue")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var perr platformerrors.PlatformError
		if !errors.As(err, &perr) {
			t.Fatalf("expected PlatformError, got %T", err)
		}

		errCtx := perr.Context()
		if errCtx == nil {
			t.Fatal("expected error context, got nil")
		}

		if filename, ok := errCtx["filename"]; !ok || filename != "test.cue" {
			t.Errorf("expected filename='test.cue' in context, got %v", filename)
		}
	})
}

// TestIntegrationScenarios tests real-world usage scenarios.
func TestIntegrationScenarios(t *testing.T) {
	t.Run("load and validate schema pattern", func(t *testing.T) {
		mfs := billy.NewMemory()

		// Create a unified schema and data file
		if err := mfs.WriteFile("config.cue", []byte(`
package config

#Config: {
	name: string
	port: int & >0 & <65536
}

config: #Config & {
	name: "myapp"
	port: 8080
}
		`), 0644); err != nil {
			t.Fatalf("failed to create config.cue: %v", err)
		}

		ctx := context.Background()
		loader := NewLoader(mfs)

		// Load the file
		result, err := loader.LoadFile(ctx, "config.cue")
		if err != nil {
			t.Fatalf("LoadFile failed: %v", err)
		}

		// Extract config
		configField := result.LookupPath(cue.ParsePath("config"))
		if configField.Err() != nil {
			t.Fatalf("failed to lookup config: %v", configField.Err())
		}

		nameField := configField.LookupPath(cue.ParsePath("name"))
		name, err := nameField.String()
		if err != nil {
			t.Fatalf("failed to get name: %v", err)
		}

		if name != "myapp" {
			t.Errorf("expected name='myapp', got %q", name)
		}
	})
}
