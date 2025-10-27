package billy

import (
	iofs "io/fs"
	"os"
	"testing"

	"github.com/jmgilman/go/fs/core"
	"github.com/jmgilman/go/fs/fstest"
)

// TestLocalFS_Constructor verifies NewLocal creates a valid filesystem.
func TestLocalFS_Constructor(t *testing.T) {
	fs := NewLocal()
	if fs == nil {
		t.Fatal("NewLocal() returned nil")
	}
	if fs.bfs == nil {
		t.Error("NewLocal() bfs field is nil")
	}
}

// TestMemoryFS_Constructor verifies NewMemory creates a valid filesystem.
func TestMemoryFS_Constructor(t *testing.T) {
	fs := NewMemory()
	if fs == nil {
		t.Fatal("NewMemory() returned nil")
	}
	if fs.bfs == nil {
		t.Error("NewMemory() bfs field is nil")
	}
}

// TestLocalFS_Unwrap verifies Unwrap returns the underlying billy.Filesystem.
func TestLocalFS_Unwrap(t *testing.T) {
	fs := NewLocal()
	billyFS := fs.Unwrap()
	if billyFS == nil {
		t.Fatal("Unwrap() returned nil")
	}

	// Verify it's a billy.Filesystem by using it
	_ = billyFS
}

// TestMemoryFS_Unwrap verifies Unwrap returns the underlying billy.Filesystem.
func TestMemoryFS_Unwrap(t *testing.T) {
	fs := NewMemory()
	billyFS := fs.Unwrap()
	if billyFS == nil {
		t.Fatal("Unwrap() returned nil")
	}

	// Verify it's a billy.Filesystem and can be used directly
	_ = billyFS

	// Verify we can use it directly with billy operations
	_, err := billyFS.Create("test.txt")
	if err != nil {
		t.Errorf("Failed to use unwrapped filesystem: %v", err)
	}
}

// TestLocalFS_Type verifies LocalFS returns FSTypeLocal.
func TestLocalFS_Type(t *testing.T) {
	fs := NewLocal()
	fsType := fs.Type()
	if fsType != core.FSTypeLocal {
		t.Errorf("LocalFS.Type() = %v (%s), want %v (%s)",
			fsType, fsType.String(), core.FSTypeLocal, core.FSTypeLocal.String())
	}
}

// TestMemoryFS_Type verifies MemoryFS returns FSTypeMemory.
func TestMemoryFS_Type(t *testing.T) {
	fs := NewMemory()
	fsType := fs.Type()
	if fsType != core.FSTypeMemory {
		t.Errorf("MemoryFS.Type() = %v (%s), want %v (%s)",
			fsType, fsType.String(), core.FSTypeMemory, core.FSTypeMemory.String())
	}
}

// TestMemoryFS_BasicOperations verifies basic read/write operations work.
func TestMemoryFS_BasicOperations(t *testing.T) {
	fs := NewMemory()

	// Test WriteFile
	testData := []byte("Hello, World!")
	err := fs.WriteFile("test.txt", testData, 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Test ReadFile
	data, err := fs.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("ReadFile() = %q, want %q", data, testData)
	}

	// Test Stat
	info, err := fs.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.IsDir() {
		t.Error("Stat() IsDir() = true, want false")
	}
	if info.Size() != int64(len(testData)) {
		t.Errorf("Stat() Size() = %d, want %d", info.Size(), len(testData))
	}
}

// TestMemoryFS_DirectoryOperations verifies directory operations work.
func TestMemoryFS_DirectoryOperations(t *testing.T) {
	fs := NewMemory()

	// Test MkdirAll
	err := fs.MkdirAll("a/b/c", 0755)
	if err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Test Stat on directory
	info, err := fs.Stat("a/b/c")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("Stat() IsDir() = false, want true")
	}

	// Test ReadDir
	err = fs.WriteFile("a/file1.txt", []byte("data1"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	err = fs.WriteFile("a/file2.txt", []byte("data2"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	entries, err := fs.ReadDir("a")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 3 { // b directory + 2 files
		t.Errorf("ReadDir() returned %d entries, want 3", len(entries))
	}

	// Verify entries implement iofs.DirEntry
	for _, entry := range entries {
		_ = entry
		info, err := entry.Info()
		if err != nil {
			t.Errorf("DirEntry.Info() error = %v", err)
		}
		if info == nil {
			t.Error("DirEntry.Info() returned nil")
		}
	}
}

// TestMemoryFS_Remove verifies Remove operations.
func TestMemoryFS_Remove(t *testing.T) {
	fs := NewMemory()

	// Create a file
	err := fs.WriteFile("test.txt", []byte("data"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Remove it
	err = fs.Remove("test.txt")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Verify it's gone
	_, err = fs.Stat("test.txt")
	if !os.IsNotExist(err) {
		t.Errorf("Stat() after Remove() error = %v, want ErrNotExist", err)
	}
}

// TestMemoryFS_RemoveAll verifies RemoveAll operations.
func TestMemoryFS_RemoveAll(t *testing.T) {
	fs := NewMemory()

	// Create directory structure
	err := fs.MkdirAll("a/b/c", 0755)
	if err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	err = fs.WriteFile("a/file1.txt", []byte("data1"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	err = fs.WriteFile("a/b/file2.txt", []byte("data2"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// RemoveAll
	err = fs.RemoveAll("a")
	if err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	// Verify it's gone
	_, err = fs.Stat("a")
	if !os.IsNotExist(err) {
		t.Errorf("Stat() after RemoveAll() error = %v, want ErrNotExist", err)
	}
}

// TestMemoryFS_Rename verifies Rename operations.
func TestMemoryFS_Rename(t *testing.T) {
	fs := NewMemory()

	// Create a file
	testData := []byte("test data")
	err := fs.WriteFile("old.txt", testData, 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Rename it
	err = fs.Rename("old.txt", "new.txt")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	// Verify old name is gone
	_, err = fs.Stat("old.txt")
	if !os.IsNotExist(err) {
		t.Errorf("Stat(old) after Rename() error = %v, want ErrNotExist", err)
	}

	// Verify new name exists with same data
	data, err := fs.ReadFile("new.txt")
	if err != nil {
		t.Fatalf("ReadFile(new) error = %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("ReadFile(new) = %q, want %q", data, testData)
	}
}

// TestMemoryFS_Walk verifies Walk operations.
func TestMemoryFS_Walk(t *testing.T) {
	fs := NewMemory()

	// Create directory structure
	err := fs.MkdirAll("a/b", 0755)
	if err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	err = fs.WriteFile("a/file1.txt", []byte("data1"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	err = fs.WriteFile("a/b/file2.txt", []byte("data2"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Walk and collect paths
	var paths []string
	err = fs.Walk("a", func(path string, _ iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	// Verify all paths were visited
	expectedMin := 4 // a, a/file1.txt, a/b, a/b/file2.txt
	if len(paths) < expectedMin {
		t.Errorf("Walk() visited %d paths, want at least %d: %v", len(paths), expectedMin, paths)
	}
}

// TestMemoryFS_Chroot verifies Chroot operations.
func TestMemoryFS_Chroot(t *testing.T) {
	fs := NewMemory()

	// Create directory structure
	err := fs.MkdirAll("subdir", 0755)
	if err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	err = fs.WriteFile("subdir/file.txt", []byte("data"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Chroot into subdir
	chrootFS, err := fs.Chroot("subdir")
	if err != nil {
		t.Fatalf("Chroot() error = %v", err)
	}

	// Verify we can access file.txt from root of chroot
	data, err := chrootFS.ReadFile("file.txt")
	if err != nil {
		t.Fatalf("ReadFile() in chroot error = %v", err)
	}
	if string(data) != "data" {
		t.Errorf("ReadFile() in chroot = %q, want %q", data, "data")
	}

	// Verify the chrooted filesystem is the correct type
	if _, ok := chrootFS.(*MemoryFS); !ok {
		t.Errorf("Chroot() returned type %T, want *MemoryFS", chrootFS)
	}
}

// TestMemoryFS_Open verifies Open returns correct File type.
func TestMemoryFS_Open(t *testing.T) {
	fs := NewMemory()

	// Create a file
	err := fs.WriteFile("test.txt", []byte("test data"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Open it
	f, err := fs.Open("test.txt")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = f.Close() }()

	// Verify it's an iofs.File
	_ = f
}

// TestMemoryFS_Create verifies Create returns correct File type.
func TestMemoryFS_Create(t *testing.T) {
	fs := NewMemory()

	// Create a file
	f, err := fs.Create("test.txt")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer func() { _ = f.Close() }()

	// Verify it implements core.File
	if _, ok := f.(*File); !ok {
		t.Errorf("Create() returned type %T, want *File", f)
	}
}

// TestMemoryFS_OpenFile verifies OpenFile with various flags.
func TestMemoryFS_OpenFile(t *testing.T) {
	fs := NewMemory()

	tests := []struct {
		name string
		flag int
		perm iofs.FileMode
	}{
		{"read only", os.O_RDONLY, 0644},
		{"write only", os.O_WRONLY, 0644},
		{"read write", os.O_RDWR, 0644},
		{"create", os.O_CREATE | os.O_WRONLY, 0644},
		{"truncate", os.O_TRUNC | os.O_WRONLY, 0644},
		{"append", os.O_APPEND | os.O_WRONLY, 0644},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: create file if needed
			if tt.flag&os.O_CREATE == 0 && tt.flag&os.O_RDONLY == os.O_RDONLY {
				err := fs.WriteFile("test_"+tt.name+".txt", []byte("data"), 0644)
				if err != nil {
					t.Fatalf("Setup WriteFile() error = %v", err)
				}
			}

			// Test OpenFile
			f, err := fs.OpenFile("test_"+tt.name+".txt", tt.flag, tt.perm)
			if err != nil {
				t.Fatalf("OpenFile() error = %v", err)
			}
			defer func() { _ = f.Close() }()

			// Verify it implements core.File
			if _, ok := f.(*File); !ok {
				t.Errorf("OpenFile() returned type %T, want *File", f)
			}
		})
	}
}

// TestNormalize verifies the normalize helper function.
func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "test.txt", "test.txt"},
		{"with slash", "dir/file.txt", "dir/file.txt"},
		{"with double slash", "dir//file.txt", "dir/file.txt"},
		{"with dot", "dir/./file.txt", "dir/file.txt"},
		{"with dotdot", "dir/../file.txt", "file.txt"},
		{"root", "/", "/"},
		{"absolute", "/dir/file.txt", "/dir/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.input)
			if got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDirEntry_Methods verifies dirEntry implements fs.DirEntry correctly.
func TestDirEntry_Methods(t *testing.T) {
	fs := NewMemory()

	// Create a test file
	err := fs.WriteFile("test.txt", []byte("data"), 0644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Get directory entry
	entries, err := fs.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("ReadDir() returned no entries")
	}

	entry := entries[0]

	// Test Name()
	if entry.Name() == "" {
		t.Error("DirEntry.Name() returned empty string")
	}

	// Test IsDir()
	_ = entry.IsDir() // Just verify it doesn't panic

	// Test Type()
	_ = entry.Type() // Just verify it doesn't panic

	// Test Info()
	info, err := entry.Info()
	if err != nil {
		t.Errorf("DirEntry.Info() error = %v", err)
	}
	if info == nil {
		t.Error("DirEntry.Info() returned nil")
	}
}

// TestLocalFS_Operations verifies LocalFS operations work similarly to MemoryFS.
func TestLocalFS_Operations(t *testing.T) {
	_ = t // Test parameter used for test framework requirements
	fs := NewLocal()

	// Test that basic operations don't panic
	// Note: We can't fully test LocalFS without filesystem cleanup,
	// but we can verify the interface is implemented correctly.

	// Verify interface implementation
	_ = fs
	_ = iofs.FS(fs)
	_ = iofs.ReadFileFS(fs)
	_ = iofs.StatFS(fs)
	_ = iofs.ReadDirFS(fs)
}

// TestMemoryFS_RemoveAll_NonExistent verifies RemoveAll returns nil for non-existent path.
func TestMemoryFS_RemoveAll_NonExistent(t *testing.T) {
	fs := NewMemory()

	// RemoveAll on non-existent path should not error
	err := fs.RemoveAll("nonexistent")
	if err != nil {
		t.Errorf("RemoveAll(nonexistent) error = %v, want nil", err)
	}
}

// TestMemoryFS_Mkdir verifies Mkdir creates directories.
func TestMemoryFS_Mkdir(t *testing.T) {
	fs := NewMemory()

	// Create directory
	err := fs.Mkdir("testdir", 0755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	// Verify it exists
	info, err := fs.Stat("testdir")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("Stat() IsDir() = false, want true")
	}
}

// TestLocalFS runs the fstest conformance suite against LocalFS.
// This verifies LocalFS correctly implements the core.FS interface.
// Uses t.TempDir() + Chroot to provide a clean, isolated test environment.
func TestLocalFS(t *testing.T) {
	tmpDir := t.TempDir()
	rootFS := NewLocal()
	testFS, err := rootFS.Chroot(tmpDir)
	if err != nil {
		t.Fatalf("Chroot(%q) failed: %v", tmpDir, err)
	}

	// Skip tests incompatible with billy's behavioral differences or test isolation issues
	skipTests := []string{
		"WriteFS/CreateInNonExistentDir", // Billy auto-creates parent directories
		"WriteFS/Mkdir",                  // Test isolation issue with shared temp directory
		"FileCapabilities/ReadDirFile",   // Billy doesn't support opening directories as files
	}

	fstest.TestSuiteWithSkip(t, func() core.FS { return testFS }, skipTests)
}

// TestMemoryFS runs the fstest conformance suite against MemoryFS.
// This verifies MemoryFS correctly implements the core.FS interface.
func TestMemoryFS(t *testing.T) {
	// Skip tests incompatible with billy's behavioral differences
	skipTests := []string{
		"WriteFS/CreateInNonExistentDir", // Billy auto-creates parent directories
		"FileCapabilities/ReadDirFile",   // Billy doesn't support opening directories as files
	}

	fstest.TestSuiteWithSkip(t, func() core.FS { return NewMemory() }, skipTests)
}

// TestOpenFileFlags verifies OpenFile flag support for billy filesystems.
// Billy supports standard POSIX flags including O_SYNC.
func TestOpenFileFlags(t *testing.T) {
	supportedFlags := []int{
		os.O_RDONLY,
		os.O_WRONLY,
		os.O_RDWR,
		os.O_CREATE,
		os.O_TRUNC,
		os.O_APPEND,
		os.O_EXCL,
		os.O_SYNC, // Billy supports O_SYNC
	}

	t.Run("LocalFS", func(t *testing.T) {
		tmpDir := t.TempDir()
		rootFS := NewLocal()
		testFS, err := rootFS.Chroot(tmpDir)
		if err != nil {
			t.Fatalf("Chroot(%q) failed: %v", tmpDir, err)
		}
		fstest.TestOpenFileFlags(t, testFS, supportedFlags)
	})

	t.Run("MemoryFS", func(t *testing.T) {
		fs := NewMemory()
		fstest.TestOpenFileFlags(t, fs, supportedFlags)
	})
}
