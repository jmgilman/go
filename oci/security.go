// Package ocibundle provides OCI bundle distribution functionality.
// This file contains security validators and constraints for safe archive handling.
package ocibundle

import "fmt"

// Validator checks for security issues during archive extraction.
// Implementations of this interface validate different aspects of files and archives
// to prevent security vulnerabilities such as path traversal attacks, zip bombs,
// and other malicious archive content.
type Validator interface {
	// ValidatePath checks if a file path is safe for extraction.
	// This prevents path traversal attacks by rejecting paths containing ".."
	// or absolute paths that could escape the extraction directory.
	ValidatePath(path string) error

	// ValidateFile checks if a file's properties are acceptable.
	// This includes size limits, permission checks, and other file-specific validations.
	ValidateFile(info FileInfo) error

	// ValidateArchive checks if archive statistics are within acceptable limits.
	// This prevents zip bomb attacks by enforcing total file count and size limits.
	ValidateArchive(stats ArchiveStats) error
}

// FileInfo represents file information used for security validation.
// This struct contains the essential metadata needed to validate files
// during archive extraction without requiring access to the actual file content.
type FileInfo struct {
	// Name is the file path within the archive
	Name string

	// Size is the uncompressed size of the file in bytes
	Size int64

	// Mode contains the file permissions and type information
	Mode uint32
}

// ArchiveStats represents archive statistics used for security validation.
// This struct provides summary information about the entire archive
// to enable validation of aggregate limits and constraints.
type ArchiveStats struct {
	// TotalFiles is the total number of files in the archive
	TotalFiles int

	// TotalSize is the total uncompressed size of all files in bytes
	TotalSize int64
}

// SizeValidator enforces size limits on files and archives.
// It prevents resource exhaustion attacks by limiting individual file sizes
// and total archive sizes.
type SizeValidator struct {
	// MaxFileSize is the maximum allowed size for any individual file (in bytes).
	// Set to 0 to disable individual file size limits.
	MaxFileSize int64

	// MaxTotalSize is the maximum allowed total size for all files combined (in bytes).
	// Set to 0 to disable total size limits.
	MaxTotalSize int64
}

// NewSizeValidator creates a new SizeValidator with the specified limits.
func NewSizeValidator(maxFileSize, maxTotalSize int64) *SizeValidator {
	return &SizeValidator{
		MaxFileSize:  maxFileSize,
		MaxTotalSize: maxTotalSize,
	}
}

// ValidatePath is a no-op for SizeValidator since it doesn't validate paths.
func (v *SizeValidator) ValidatePath(path string) error {
	return nil
}

// ValidateFile checks if a file's size is within acceptable limits.
func (v *SizeValidator) ValidateFile(info FileInfo) error {
	if v.MaxFileSize > 0 && info.Size > v.MaxFileSize {
		return &BundleError{
			Op:        "validate",
			Reference: info.Name,
			Err:       ErrSecurityViolation,
		}
	}
	return nil
}

// ValidateArchive checks if the total archive size is within acceptable limits.
func (v *SizeValidator) ValidateArchive(stats ArchiveStats) error {
	if v.MaxTotalSize > 0 && stats.TotalSize > v.MaxTotalSize {
		return &BundleError{
			Op:        "validate",
			Reference: "archive",
			Err:       ErrSecurityViolation,
		}
	}
	return nil
}

// FileCountValidator prevents zip bomb attacks by limiting the number of files.
type FileCountValidator struct {
	// MaxFiles is the maximum number of files allowed in an archive.
	// Set to 0 to disable file count limits.
	MaxFiles int
}

// NewFileCountValidator creates a new FileCountValidator with the specified limit.
func NewFileCountValidator(maxFiles int) *FileCountValidator {
	return &FileCountValidator{
		MaxFiles: maxFiles,
	}
}

// ValidatePath is a no-op for FileCountValidator since it doesn't validate paths.
func (v *FileCountValidator) ValidatePath(path string) error {
	return nil
}

// ValidateFile is a no-op for FileCountValidator since it validates at archive level.
func (v *FileCountValidator) ValidateFile(info FileInfo) error {
	return nil
}

// ValidateArchive checks if the file count is within acceptable limits.
func (v *FileCountValidator) ValidateArchive(stats ArchiveStats) error {
	if v.MaxFiles > 0 && stats.TotalFiles > v.MaxFiles {
		return &BundleError{
			Op:        "validate",
			Reference: "archive",
			Err:       ErrSecurityViolation,
		}
	}
	return nil
}

// PermissionSanitizer removes dangerous permission bits from files.
// It prevents privilege escalation by removing setuid and setgid bits.
type PermissionSanitizer struct{}

// NewPermissionSanitizer creates a new PermissionSanitizer.
func NewPermissionSanitizer() *PermissionSanitizer {
	return &PermissionSanitizer{}
}

// ValidatePath is a no-op for PermissionSanitizer since it doesn't validate paths.
func (v *PermissionSanitizer) ValidatePath(path string) error {
	return nil
}

// ValidateFile checks and sanitizes file permissions.
// It removes setuid and setgid bits to prevent privilege escalation.
func (v *PermissionSanitizer) ValidateFile(info FileInfo) error {
	// Check for setuid bit (04000)
	if info.Mode&0o4000 != 0 {
		return &BundleError{
			Op:        "validate",
			Reference: info.Name,
			Err:       ErrSecurityViolation,
		}
	}

	// Check for setgid bit (02000)
	if info.Mode&0o2000 != 0 {
		return &BundleError{
			Op:        "validate",
			Reference: info.Name,
			Err:       ErrSecurityViolation,
		}
	}

	return nil
}

// ValidateArchive is a no-op for PermissionSanitizer since it validates at file level.
func (v *PermissionSanitizer) ValidateArchive(stats ArchiveStats) error {
	return nil
}

// SanitizePermissions removes dangerous permission bits from a file mode.
// This is a utility function that can be used during extraction.
func (v *PermissionSanitizer) SanitizePermissions(mode uint32) uint32 {
	// Remove setuid and setgid bits
	return mode &^ 0o4000 &^ 0o2000
}

// ValidatorChain combines multiple validators and executes them in sequence.
// It fails fast - returns the first validation error encountered.
type ValidatorChain struct {
	validators []Validator
}

// NewValidatorChain creates a new ValidatorChain with the specified validators.
func NewValidatorChain(validators ...Validator) *ValidatorChain {
	return &ValidatorChain{
		validators: validators,
	}
}

// AddValidator adds a validator to the chain.
func (vc *ValidatorChain) AddValidator(validator Validator) {
	vc.validators = append(vc.validators, validator)
}

// ValidatePath runs all validators' ValidatePath methods in sequence.
// Returns the first error encountered, or nil if all pass.
func (vc *ValidatorChain) ValidatePath(path string) error {
	for _, validator := range vc.validators {
		if err := validator.ValidatePath(path); err != nil {
			return fmt.Errorf("path validation failed for %s: %w", path, err)
		}
	}
	return nil
}

// ValidateFile runs all validators' ValidateFile methods in sequence.
// Returns the first error encountered, or nil if all pass.
func (vc *ValidatorChain) ValidateFile(info FileInfo) error {
	for _, validator := range vc.validators {
		if err := validator.ValidateFile(info); err != nil {
			return fmt.Errorf("file validation failed for %s: %w", info.Name, err)
		}
	}
	return nil
}

// ValidateArchive runs all validators' ValidateArchive methods in sequence.
// Returns the first error encountered, or nil if all pass.
func (vc *ValidatorChain) ValidateArchive(stats ArchiveStats) error {
	for _, validator := range vc.validators {
		if err := validator.ValidateArchive(stats); err != nil {
			return fmt.Errorf(
				"archive validation failed (files: %d, size: %d): %w",
				stats.TotalFiles,
				stats.TotalSize,
				err,
			)
		}
	}
	return nil
}
