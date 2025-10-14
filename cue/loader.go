package cue

import (
	"context"
	"fmt"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/jmgilman/go/fs/core"
)

// Loader manages CUE loading operations from a filesystem.
// It maintains a CUE context for compilation and provides methods to load
// CUE files, packages, and modules using proper CUE semantics.
type Loader struct {
	fs     core.ReadFS
	cueCtx *cue.Context
}

// NewLoader creates a new CUE loader with the given filesystem.
// The loader manages its own CUE context for compilation operations.
func NewLoader(filesystem core.ReadFS) *Loader {
	return &Loader{
		fs:     filesystem,
		cueCtx: cuecontext.New(),
	}
}

// Context returns the underlying CUE context.
// This can be used for advanced CUE operations that need direct access to the context.
func (l *Loader) Context() *cue.Context {
	return l.cueCtx
}

// LoadFile loads a single CUE file from the filesystem.
// The filePath is relative to the filesystem root.
//
// Returns CodeCUELoadFailed on file I/O errors.
// Returns CodeCUEBuildFailed on CUE compilation errors.
func (l *Loader) LoadFile(ctx context.Context, filePath string) (cue.Value, error) {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(err, "context cancelled", makeContext("file_path", filePath))
	}

	// Create overlay with single file
	overlay, err := l.buildOverlayForFiles([]string{filePath})
	if err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(
			err,
			"failed to read CUE file",
			makeContext("file_path", filePath),
		)
	}

	// Load using load.Instances
	config := &load.Config{
		Dir:     "/",
		Overlay: overlay,
	}

	insts := load.Instances([]string{filePath}, config)
	if len(insts) == 0 {
		return cue.Value{}, wrapLoadErrorWithContext(
			fmt.Errorf("no instances loaded"),
			"failed to load CUE file",
			makeContext("file_path", filePath),
		)
	}

	if err := insts[0].Err; err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"failed to load CUE file",
			makeContext("file_path", filePath),
		)
	}

	// Build the instance
	val := l.cueCtx.BuildInstance(insts[0])
	if err := val.Err(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"failed to build CUE file",
			makeContext("file_path", filePath),
		)
	}

	// Validate the result
	if err := val.Validate(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"CUE validation failed",
			makeContext("file_path", filePath),
		)
	}

	return val, nil
}

// LoadPackage loads all CUE files in a directory as a single package.
// Only files in the immediate directory are loaded (non-recursive).
// The packagePath is relative to the filesystem root.
//
// Returns CodeCUELoadFailed on file I/O errors.
// Returns CodeCUEBuildFailed on CUE compilation errors.
func (l *Loader) LoadPackage(ctx context.Context, packagePath string) (cue.Value, error) {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(err, "context cancelled", makeContext("package_path", packagePath))
	}

	// Discover CUE files in the package directory (non-recursive)
	entries, err := l.fs.ReadDir(packagePath)
	if err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(
			err,
			"failed to read package directory",
			makeContext("package_path", packagePath),
		)
	}

	// Filter for .cue files
	var filePaths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".cue" {
			filePaths = append(filePaths, filepath.Join(packagePath, entry.Name()))
		}
	}

	if len(filePaths) == 0 {
		return cue.Value{}, wrapLoadErrorWithContext(
			fmt.Errorf("no CUE files found"),
			"package contains no .cue files",
			makeContext("package_path", packagePath),
		)
	}

	// Create overlay with all files in the package
	overlay, err := l.buildOverlayForFiles(filePaths)
	if err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(
			err,
			"failed to read package files",
			makeContext("package_path", packagePath),
		)
	}

	// Load using load.Instances
	config := &load.Config{
		Dir:     "/" + packagePath,
		Overlay: overlay,
	}

	insts := load.Instances([]string{"."}, config)
	if len(insts) == 0 {
		return cue.Value{}, wrapLoadErrorWithContext(
			fmt.Errorf("no instances loaded"),
			"failed to load package",
			makeContext("package_path", packagePath),
		)
	}

	if err := insts[0].Err; err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"failed to load package",
			makeContext("package_path", packagePath),
		)
	}

	// Build the instance
	val := l.cueCtx.BuildInstance(insts[0])
	if err := val.Err(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"failed to build package",
			makeContext("package_path", packagePath),
		)
	}

	// Validate the result
	if err := val.Validate(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"CUE validation failed",
			makeContext("package_path", packagePath),
		)
	}

	return val, nil
}

// LoadModule loads a CUE module with proper package structure.
// This recursively discovers all .cue files in the module and respects package organization.
// The modulePath is relative to the filesystem root and should point to the module root.
//
// Returns CodeCUELoadFailed on file I/O errors.
// Returns CodeCUEBuildFailed on CUE compilation errors.
func (l *Loader) LoadModule(ctx context.Context, modulePath string) (cue.Value, error) {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(err, "context cancelled", makeContext("module_path", modulePath))
	}

	// Recursively discover all CUE files in the module
	filePaths, err := discoverCueFiles(l.fs, modulePath)
	if err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(
			err,
			"failed to discover CUE files in module",
			makeContext("module_path", modulePath),
		)
	}

	if len(filePaths) == 0 {
		return cue.Value{}, wrapLoadErrorWithContext(
			fmt.Errorf("no CUE files found"),
			"module contains no .cue files",
			makeContext("module_path", modulePath),
		)
	}

	// Create overlay with all files in the module
	overlay, err := l.buildOverlayForFiles(filePaths)
	if err != nil {
		return cue.Value{}, wrapLoadErrorWithContext(
			err,
			"failed to read module files",
			makeContext("module_path", modulePath),
		)
	}

	// Load using load.Instances
	config := &load.Config{
		Dir:        "/" + modulePath,
		ModuleRoot: "/" + modulePath,
		Overlay:    overlay,
	}

	insts := load.Instances([]string{"."}, config)
	if len(insts) == 0 {
		return cue.Value{}, wrapLoadErrorWithContext(
			fmt.Errorf("no instances loaded"),
			"failed to load module",
			makeContext("module_path", modulePath),
		)
	}

	if err := insts[0].Err; err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"failed to load module",
			makeContext("module_path", modulePath),
		)
	}

	// Build the instance
	val := l.cueCtx.BuildInstance(insts[0])
	if err := val.Err(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"failed to build module",
			makeContext("module_path", modulePath),
		)
	}

	// Validate the result
	if err := val.Validate(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"CUE validation failed",
			makeContext("module_path", modulePath),
		)
	}

	return val, nil
}

// LoadBytes loads CUE source from byte slices with optional filename for error reporting.
// The filename parameter is used only for error messages and can be empty or synthetic.
// This method is useful for testing or loading dynamically generated CUE content.
//
// Returns CodeCUEBuildFailed on CUE compilation errors.
func (l *Loader) LoadBytes(ctx context.Context, source []byte, filename string) (cue.Value, error) {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(err, "context cancelled", makeContext("filename", filename))
	}

	if filename == "" {
		filename = "<input>"
	}

	// Compile the source directly
	val := l.cueCtx.CompileBytes(source, cue.Filename(filename))
	if err := val.Err(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"failed to compile CUE source",
			makeContext("filename", filename, "source_size", len(source)),
		)
	}

	// Validate the compiled value
	if err := val.Validate(); err != nil {
		return cue.Value{}, wrapBuildErrorWithContext(
			err,
			"CUE validation failed",
			makeContext("filename", filename),
		)
	}

	return val, nil
}

// buildOverlayForFiles creates a load.Config overlay from a list of file paths.
// The overlay maps absolute paths to load.Source for use with load.Instances.
func (l *Loader) buildOverlayForFiles(filePaths []string) (map[string]load.Source, error) {
	overlay := make(map[string]load.Source)

	for _, path := range filePaths {
		data, err := l.fs.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Ensure path starts with /
		absPath := path
		if absPath[0] != '/' {
			absPath = "/" + absPath
		}

		overlay[absPath] = load.FromBytes(data)
	}

	return overlay, nil
}

// discoverCueFiles recursively discovers .cue files in a directory.
// This is a helper for LoadModule and is not exported.
func discoverCueFiles(filesystem core.ReadFS, dir string) ([]string, error) {
	var files []string

	var walk func(string) error
	walk = func(path string) error {
		entries, err := filesystem.ReadDir(path)
		if err != nil {
			return fmt.Errorf("failed to read directory %s: %w", path, err)
		}

		for _, entry := range entries {
			entryPath := filepath.Join(path, entry.Name())

			if entry.IsDir() {
				// Recursively walk subdirectories
				if err := walk(entryPath); err != nil {
					return err
				}
			} else if filepath.Ext(entry.Name()) == ".cue" {
				files = append(files, entryPath)
			}
		}

		return nil
	}

	if err := walk(dir); err != nil {
		return nil, err
	}

	return files, nil
}

// fileExists checks if a file exists in the filesystem.
// This is a helper for validation and is not exported.
func fileExists(filesystem core.ReadFS, path string) (bool, error) {
	exists, err := filesystem.Exists(path)
	if err != nil {
		return false, fmt.Errorf("failed to check if file exists: %w", err)
	}
	return exists, nil
}
