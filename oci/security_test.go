package ocibundle

import (
	"errors"
	"testing"
)

// TestSizeValidator tests the SizeValidator functionality
func TestSizeValidator(t *testing.T) {
	// Test constructor
	validator := NewSizeValidator(1000, 5000)
	if validator.MaxFileSize != 1000 {
		t.Errorf("Expected MaxFileSize to be 1000, got %d", validator.MaxFileSize)
	}
	if validator.MaxTotalSize != 5000 {
		t.Errorf("Expected MaxTotalSize to be 5000, got %d", validator.MaxTotalSize)
	}

	// Test ValidatePath (should be no-op)
	err := validator.ValidatePath("test/file.txt")
	if err != nil {
		t.Errorf("Expected ValidatePath to return nil, got %v", err)
	}

	// Test ValidateFile with file within limits
	fileInfo := FileInfo{Name: "test.txt", Size: 500, Mode: 0o644}
	err = validator.ValidateFile(fileInfo)
	if err != nil {
		t.Errorf("Expected file within limits to pass, got %v", err)
	}

	// Test ValidateFile with file exceeding limits
	largeFileInfo := FileInfo{Name: "large.txt", Size: 1500, Mode: 0o644}
	err = validator.ValidateFile(largeFileInfo)
	if err == nil {
		t.Error("Expected large file to be rejected")
	}
	var bundleErr *BundleError
	if errors.As(err, &bundleErr) {
		if bundleErr.Op != "validate" {
			t.Errorf("Expected operation to be 'validate', got %q", bundleErr.Op)
		}
		if bundleErr.Reference != "large.txt" {
			t.Errorf("Expected reference to be 'large.txt', got %q", bundleErr.Reference)
		}
		if !errors.Is(bundleErr, ErrSecurityViolation) {
			t.Error("Expected ErrSecurityViolation")
		}
	}

	// Test ValidateArchive with archive within limits
	archiveStats := ArchiveStats{TotalFiles: 10, TotalSize: 3000}
	err = validator.ValidateArchive(archiveStats)
	if err != nil {
		t.Errorf("Expected archive within limits to pass, got %v", err)
	}

	// Test ValidateArchive with archive exceeding limits
	largeArchiveStats := ArchiveStats{TotalFiles: 10, TotalSize: 6000}
	err = validator.ValidateArchive(largeArchiveStats)
	if err == nil {
		t.Error("Expected large archive to be rejected")
	}
	var bundleErr2 *BundleError
	if errors.As(err, &bundleErr2) {
		if bundleErr2.Reference != "archive" {
			t.Errorf("Expected reference to be 'archive', got %q", bundleErr2.Reference)
		}
	}
}

// TestSizeValidatorDisabledLimits tests SizeValidator with disabled limits
func TestSizeValidatorDisabledLimits(t *testing.T) {
	// Test with zero limits (disabled)
	validator := NewSizeValidator(0, 0)

	// Large file should pass when limit is disabled
	largeFileInfo := FileInfo{Name: "large.txt", Size: 1000000, Mode: 0o644}
	err := validator.ValidateFile(largeFileInfo)
	if err != nil {
		t.Errorf("Expected large file to pass when limit disabled, got %v", err)
	}

	// Large archive should pass when limit is disabled
	largeArchiveStats := ArchiveStats{TotalFiles: 1000, TotalSize: 10000000}
	err = validator.ValidateArchive(largeArchiveStats)
	if err != nil {
		t.Errorf("Expected large archive to pass when limit disabled, got %v", err)
	}
}

// TestFileCountValidator tests the FileCountValidator functionality
func TestFileCountValidator(t *testing.T) {
	// Test constructor
	validator := NewFileCountValidator(100)
	if validator.MaxFiles != 100 {
		t.Errorf("Expected MaxFiles to be 100, got %d", validator.MaxFiles)
	}

	// Test ValidatePath (should be no-op)
	err := validator.ValidatePath("test/file.txt")
	if err != nil {
		t.Errorf("Expected ValidatePath to return nil, got %v", err)
	}

	// Test ValidateFile (should be no-op)
	fileInfo := FileInfo{Name: "test.txt", Size: 500, Mode: 0o644}
	err = validator.ValidateFile(fileInfo)
	if err != nil {
		t.Errorf("Expected ValidateFile to return nil, got %v", err)
	}

	// Test ValidateArchive with file count within limits
	archiveStats := ArchiveStats{TotalFiles: 50, TotalSize: 10000}
	err = validator.ValidateArchive(archiveStats)
	if err != nil {
		t.Errorf("Expected archive within limits to pass, got %v", err)
	}

	// Test ValidateArchive with file count exceeding limits
	largeArchiveStats := ArchiveStats{TotalFiles: 150, TotalSize: 10000}
	err = validator.ValidateArchive(largeArchiveStats)
	if err == nil {
		t.Error("Expected archive with too many files to be rejected")
	}
	var bundleErr *BundleError
	if errors.As(err, &bundleErr) {
		if bundleErr.Reference != "archive" {
			t.Errorf("Expected reference to be 'archive', got %q", bundleErr.Reference)
		}
	}
}

// TestFileCountValidatorDisabled tests FileCountValidator with disabled limits
func TestFileCountValidatorDisabled(t *testing.T) {
	// Test with zero limit (disabled)
	validator := NewFileCountValidator(0)

	// Large file count should pass when limit is disabled
	largeArchiveStats := ArchiveStats{TotalFiles: 10000, TotalSize: 100000}
	err := validator.ValidateArchive(largeArchiveStats)
	if err != nil {
		t.Errorf("Expected large file count to pass when limit disabled, got %v", err)
	}
}

// TestPermissionSanitizer tests the PermissionSanitizer functionality
func TestPermissionSanitizer(t *testing.T) {
	validator := NewPermissionSanitizer()

	// Test ValidatePath (should be no-op)
	err := validator.ValidatePath("test/file.txt")
	if err != nil {
		t.Errorf("Expected ValidatePath to return nil, got %v", err)
	}

	// Test ValidateArchive (should be no-op)
	archiveStats := ArchiveStats{TotalFiles: 10, TotalSize: 10000}
	err = validator.ValidateArchive(archiveStats)
	if err != nil {
		t.Errorf("Expected ValidateArchive to return nil, got %v", err)
	}

	// Test file with normal permissions (should pass)
	normalFileInfo := FileInfo{Name: "normal.txt", Size: 1000, Mode: 0o644}
	err = validator.ValidateFile(normalFileInfo)
	if err != nil {
		t.Errorf("Expected normal file to pass, got %v", err)
	}

	// Test file with setuid bit (should fail)
	setuidFileInfo := FileInfo{Name: "setuid.txt", Size: 1000, Mode: 0o4644} // 04000 + 0644
	err = validator.ValidateFile(setuidFileInfo)
	if err == nil {
		t.Error("Expected file with setuid bit to be rejected")
	}
	var bundleErr *BundleError
	if errors.As(err, &bundleErr) {
		if bundleErr.Reference != "setuid.txt" {
			t.Errorf("Expected reference to be 'setuid.txt', got %q", bundleErr.Reference)
		}
	}

	// Test file with setgid bit (should fail)
	setgidFileInfo := FileInfo{Name: "setgid.txt", Size: 1000, Mode: 0o2644} // 02000 + 0644
	err = validator.ValidateFile(setgidFileInfo)
	if err == nil {
		t.Error("Expected file with setgid bit to be rejected")
	}

	// Test file with both setuid and setgid bits (should fail)
	bothFileInfo := FileInfo{Name: "both.txt", Size: 1000, Mode: 0o6644} // 06000 + 0644
	err = validator.ValidateFile(bothFileInfo)
	if err == nil {
		t.Error("Expected file with both setuid and setgid bits to be rejected")
	}
}

// TestPermissionSanitizerSanitizePermissions tests the permission sanitization utility
func TestPermissionSanitizerSanitizePermissions(t *testing.T) {
	sanitizer := NewPermissionSanitizer()

	// Test normal permissions (should remain unchanged)
	normalMode := uint32(0o644)
	sanitized := sanitizer.SanitizePermissions(normalMode)
	if sanitized != normalMode {
		t.Errorf("Expected normal permissions to remain unchanged, got %o", sanitized)
	}

	// Test setuid bit removal
	setuidMode := uint32(0o4644) // 04000 + 0644
	sanitized = sanitizer.SanitizePermissions(setuidMode)
	expected := uint32(0o644) // setuid bit removed
	if sanitized != expected {
		t.Errorf("Expected setuid bit to be removed, got %o, expected %o", sanitized, expected)
	}

	// Test setgid bit removal
	setgidMode := uint32(0o2644) // 02000 + 0644
	sanitized = sanitizer.SanitizePermissions(setgidMode)
	expected = uint32(0o644) // setgid bit removed
	if sanitized != expected {
		t.Errorf("Expected setgid bit to be removed, got %o, expected %o", sanitized, expected)
	}

	// Test both bits removal
	bothMode := uint32(0o6644) // 06000 + 0644
	sanitized = sanitizer.SanitizePermissions(bothMode)
	expected = uint32(0o644) // both bits removed
	if sanitized != expected {
		t.Errorf("Expected both setuid and setgid bits to be removed, got %o, expected %o", sanitized, expected)
	}
}

// TestValidatorChain tests the ValidatorChain functionality
func TestValidatorChain(t *testing.T) {
	// Create individual validators
	sizeValidator := NewSizeValidator(1000, 5000)
	countValidator := NewFileCountValidator(10)
	permValidator := NewPermissionSanitizer()

	// Test constructor with validators
	chain := NewValidatorChain(sizeValidator, countValidator, permValidator)
	if len(chain.validators) != 3 {
		t.Errorf("Expected chain to have 3 validators, got %d", len(chain.validators))
	}

	// Test empty constructor
	emptyChain := NewValidatorChain()
	if len(emptyChain.validators) != 0 {
		t.Errorf("Expected empty chain to have 0 validators, got %d", len(emptyChain.validators))
	}

	// Test AddValidator
	emptyChain.AddValidator(sizeValidator)
	if len(emptyChain.validators) != 1 {
		t.Errorf("Expected chain to have 1 validator after adding, got %d", len(emptyChain.validators))
	}
}

// TestValidatorChainExecutionOrder tests that ValidatorChain executes validators in order
func TestValidatorChainExecutionOrder(t *testing.T) {
	// Create validators that will fail at different points
	failingPathValidator := &mockFailingValidator{failMethod: "ValidatePath"}
	failingFileValidator := &mockFailingValidator{failMethod: "ValidateFile"}
	failingArchiveValidator := &mockFailingValidator{failMethod: "ValidateArchive"}

	sizeValidator := NewSizeValidator(1000, 5000)
	countValidator := NewFileCountValidator(10)

	// Test ValidatePath - should fail on first validator
	chain := NewValidatorChain(failingPathValidator, sizeValidator)
	err := chain.ValidatePath("test.txt")
	if err == nil {
		t.Error("Expected ValidatePath to fail on first validator")
	}

	// Test ValidateFile - should fail on first validator that implements ValidateFile
	chain = NewValidatorChain(failingFileValidator, sizeValidator)
	fileInfo := FileInfo{Name: "test.txt", Size: 2000, Mode: 0o644} // Size > 1000 limit
	err = chain.ValidateFile(fileInfo)
	if err == nil {
		t.Error("Expected ValidateFile to fail on first validator")
	}

	// Test ValidateArchive - should fail on first validator that implements ValidateArchive
	chain = NewValidatorChain(failingArchiveValidator, countValidator)
	archiveStats := ArchiveStats{TotalFiles: 50, TotalSize: 10000} // Files > 10 limit
	err = chain.ValidateArchive(archiveStats)
	if err == nil {
		t.Error("Expected ValidateArchive to fail on first validator")
	}
}

// TestValidatorChainSuccess tests successful validation through the chain
func TestValidatorChainSuccess(t *testing.T) {
	// Create validators with permissive limits
	sizeValidator := NewSizeValidator(10000, 100000) // Large limits
	countValidator := NewFileCountValidator(100)
	permValidator := NewPermissionSanitizer()

	chain := NewValidatorChain(sizeValidator, countValidator, permValidator)

	// Test successful path validation
	err := chain.ValidatePath("valid/path/file.txt")
	if err != nil {
		t.Errorf("Expected valid path to pass through chain, got %v", err)
	}

	// Test successful file validation
	fileInfo := FileInfo{Name: "valid.txt", Size: 1000, Mode: 0o644}
	err = chain.ValidateFile(fileInfo)
	if err != nil {
		t.Errorf("Expected valid file to pass through chain, got %v", err)
	}

	// Test successful archive validation
	archiveStats := ArchiveStats{TotalFiles: 5, TotalSize: 5000}
	err = chain.ValidateArchive(archiveStats)
	if err != nil {
		t.Errorf("Expected valid archive to pass through chain, got %v", err)
	}
}

// mockFailingValidator is a test helper that fails on a specific method
type mockFailingValidator struct {
	failMethod string
}

func (m *mockFailingValidator) ValidatePath(path string) error {
	if m.failMethod == "ValidatePath" {
		return errors.New("mock path validation failure")
	}
	return nil
}

func (m *mockFailingValidator) ValidateFile(info FileInfo) error {
	if m.failMethod == "ValidateFile" {
		return errors.New("mock file validation failure")
	}
	return nil
}

func (m *mockFailingValidator) ValidateArchive(stats ArchiveStats) error {
	if m.failMethod == "ValidateArchive" {
		return errors.New("mock archive validation failure")
	}
	return nil
}
