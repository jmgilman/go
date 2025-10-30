package validate

import "testing"

// FuzzValidatePath ensures PathTraversalValidator never panics and handles arbitrary input.
func FuzzValidatePath(f *testing.F) {
	// Seed with representative safe and malicious samples
	seeds := []string{
		"file.txt",
		"dir/sub/file.txt",
		"../escape.txt",
		"..\\escape.txt",
		"/etc/passwd",
		"..%2fsecret",
		"%2e%2e%2fsecret",
		"file\x00name.txt",
		".hidden/file",
		"normal name.txt",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, path string) {
		v := NewPathTraversalValidator()
		// Should never panic
		_ = v.ValidatePath(path)
	})
}
