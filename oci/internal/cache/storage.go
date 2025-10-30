package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jmgilman/go/fs/core"
)

// Storage provides atomic, corruption-resistant filesystem operations for cache storage.
// It uses core.FS for filesystem abstraction, supporting both OS and in-memory filesystems.
type Storage struct {
	fs         core.FS
	rootPath   string
	tempDir    string
	fileLocks  *sync.Map // map[string]*sync.Mutex for per-file locking
	globalLock sync.RWMutex
}

// NewStorage creates a new storage instance with the given filesystem and root path.
// The filesystem abstraction allows testing with in-memory filesystems.
func NewStorage(fs core.FS, rootPath string) (*Storage, error) {
	if fs == nil {
		return nil, fmt.Errorf("filesystem cannot be nil")
	}
	if rootPath == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}

	// Ensure root directory exists
	if err := fs.MkdirAll(rootPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	// Create temp directory for atomic operations
	tempDir := filepath.Join(rootPath, ".temp")
	if err := fs.MkdirAll(tempDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &Storage{
		fs:        fs,
		rootPath:  rootPath,
		tempDir:   tempDir,
		fileLocks: &sync.Map{},
	}, nil
}

// getFileLock returns a mutex for the given file path, creating one if necessary.
func (s *Storage) getFileLock(path string) *sync.Mutex {
	// Use sync.Map to safely store per-file locks
	lock, _ := s.fileLocks.LoadOrStore(path, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// createTempDir creates a temporary directory, using TempFS interface if available
func (s *Storage) createTempDir(dir, pattern string) (string, error) {
	if tfs, ok := s.fs.(core.TempFS); ok {
		return tfs.TempDir(dir, pattern)
	}
	// Fallback: create a unique directory manually
	// Try multiple times to handle concurrent conflicts
	for i := 0; i < 10; i++ {
		// Generate unique name using random hex string
		randomBytes := make([]byte, 8)
		hasher := sha256.New()
		hasher.Write([]byte(fmt.Sprintf("%s-%d-%d", pattern, os.Getpid(), i)))
		hasher.Write([]byte(fmt.Sprintf("%d", ^uint(0)))) // Add pseudo-random data
		copy(randomBytes, hasher.Sum(nil))
		uniqueName := pattern + hex.EncodeToString(randomBytes)
		path := filepath.Join(dir, uniqueName)

		// Try to create the directory
		err := s.fs.MkdirAll(path, 0755)
		if err == nil {
			return path, nil
		}
		// If directory already exists, try again with different name
		if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to create unique temp directory after 10 attempts")
}

// WriteAtomically writes data to a file atomically using a temporary file and rename.
// This ensures that either the complete file is written or nothing is written,
// preventing partial/corrupted files from being visible to readers.
func (s *Storage) WriteAtomically(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	fullPath := filepath.Join(s.rootPath, path)
	dir := filepath.Dir(fullPath)

	// Acquire file lock to prevent concurrent writes to the same file
	lock := s.getFileLock(fullPath)
	lock.Lock()
	defer lock.Unlock()

	// Ensure directory exists (protected by global lock)
	s.globalLock.Lock()
	err := s.fs.MkdirAll(dir, 0o755)
	s.globalLock.Unlock()
	if err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	// Create temporary file in temp directory (protected by global lock)
	s.globalLock.Lock()
	tempDirName, err := s.createTempDir(s.tempDir, "cache_")
	s.globalLock.Unlock()
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	tempFile := filepath.Join(tempDirName, "temp")

	// Write data to temp file
	err = s.writeWithChecksum(tempFile, data)
	if err != nil {
		// Clean up temp directory on error
		s.globalLock.Lock()
		_ = s.fs.Remove(tempFile) // Ignore errors
		_ = s.fs.Remove(tempDirName)
		s.globalLock.Unlock()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename: this is atomic on POSIX filesystems (protected by global lock)
	s.globalLock.Lock()
	err = s.fs.Rename(tempFile, fullPath)
	s.globalLock.Unlock()
	if err != nil {
		// Clean up temp directory on error
		s.globalLock.Lock()
		_ = s.fs.Remove(tempFile)
		_ = s.fs.Remove(tempDirName)
		s.globalLock.Unlock()
		return fmt.Errorf("failed to rename temp file to %q: %w", fullPath, err)
	}

	// Clean up temp directory after successful rename
	s.globalLock.Lock()
	_ = s.fs.Remove(tempDirName) // File was renamed, so directory should be empty
	s.globalLock.Unlock()

	return nil
}

// ReadWithIntegrity reads data from a file and verifies its integrity using checksums.
func (s *Storage) ReadWithIntegrity(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	fullPath := filepath.Join(s.rootPath, path)

	// Acquire read lock for the file
	lock := s.getFileLock(fullPath)
	lock.Lock()
	defer lock.Unlock()

	// Check if file exists (protected by global lock)
	s.globalLock.RLock()
	exists, err := s.fs.Exists(fullPath)
	s.globalLock.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("failed to check file existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("file does not exist: %s", fullPath)
	}

	return s.readWithChecksum(fullPath)
}

// Exists checks if a file exists in the storage.
func (s *Storage) Exists(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

	fullPath := filepath.Join(s.rootPath, path)
	// Protected by global lock
	s.globalLock.RLock()
	exists, err := s.fs.Exists(fullPath)
	s.globalLock.RUnlock()
	if err != nil {
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}
	return exists, nil
}

// Remove removes a file from the storage.
func (s *Storage) Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	fullPath := filepath.Join(s.rootPath, path)

	// Acquire file lock to prevent concurrent operations
	lock := s.getFileLock(fullPath)
	lock.Lock()
	defer lock.Unlock()

	// Protected by global lock
	s.globalLock.Lock()
	err := s.fs.Remove(fullPath)
	s.globalLock.Unlock()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file %q: %w", fullPath, err)
	}

	return nil
}

// ListFiles returns a list of files in the given directory path.
func (s *Storage) ListFiles(ctx context.Context, dirPath string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	fullPath := filepath.Join(s.rootPath, dirPath)

	// Check if directory exists (protected by global lock)
	s.globalLock.RLock()
	exists, err := s.fs.Exists(fullPath)
	if err != nil {
		s.globalLock.RUnlock()
		return nil, fmt.Errorf("failed to check directory existence: %w", err)
	}
	if !exists {
		s.globalLock.RUnlock()
		return []string{}, nil // Empty list for non-existent directories
	}

	entries, err := s.fs.ReadDir(fullPath)
	s.globalLock.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %w", fullPath, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// CleanupTempFiles removes any leftover temporary files from failed operations.
func (s *Storage) CleanupTempFiles(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Let cleanupTempDirectory handle its own locking to avoid deadlock
	return s.cleanupTempDirectory(s.tempDir)
}

// Size returns the total size of all files in the storage.
func (s *Storage) Size(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("context cancelled: %w", err)
	}

	var totalSize int64
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			totalSize += info.Size()
		}
		return nil
	}

	// Protected by global lock
	s.globalLock.RLock()
	err := s.fs.Walk(s.rootPath, walkFn)
	s.globalLock.RUnlock()
	if err != nil {
		return 0, fmt.Errorf("failed to calculate storage size: %w", err)
	}

	return totalSize, nil
}

// writeWithChecksum writes data to a file along with its SHA256 checksum.
func (s *Storage) writeWithChecksum(path string, data []byte) error {
	// Protected by global lock
	s.globalLock.Lock()
	file, err := s.fs.Create(path)
	s.globalLock.Unlock()
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", path, err)
	}
	defer file.Close()

	// Create a new hasher for each operation to avoid data races
	hasher := sha256.New()
	hasher.Write(data)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Write checksum as first line
	if _, err := file.Write([]byte(checksum + "\n")); err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	// Write actual data
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// readWithChecksum reads data from a file and verifies its checksum.
func (s *Storage) readWithChecksum(path string) ([]byte, error) {
	// Protected by global lock
	s.globalLock.RLock()
	file, err := s.fs.Open(path)
	s.globalLock.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q: %w", path, err)
	}
	defer file.Close()

	// Read entire file
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse checksum and data
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) != 2 {
		return nil, ErrCacheCorrupted
	}

	expectedChecksum := lines[0]
	actualData := []byte(lines[1])

	// Verify checksum using a new hasher to avoid data races
	hasher := sha256.New()
	hasher.Write(actualData)
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))

	if expectedChecksum != actualChecksum {
		return nil, ErrCacheCorrupted
	}

	return actualData, nil
}

// cleanupTempDirectory recursively removes all files and subdirectories in the temp directory.
func (s *Storage) cleanupTempDirectory(dir string) error {
	// Protected by global lock
	s.globalLock.RLock()
	exists, err := s.fs.Exists(dir)
	s.globalLock.RUnlock()
	if err != nil {
		return fmt.Errorf("failed to check temp directory: %w", err)
	}
	if !exists {
		return nil
	}

	// Protected by global lock
	s.globalLock.Lock()
	walkErr := s.fs.Walk(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Don't remove the temp directory itself, just its contents
		if path == dir {
			return nil
		}

		if err := s.fs.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove temp file %q: %w", path, err)
		}

		return nil
	})
	s.globalLock.Unlock()
	if walkErr != nil {
		return fmt.Errorf("failed to walk temp directory: %w", walkErr)
	}
	return nil
}

// StreamWriter provides streaming write operations with integrity verification.
type StreamWriter struct {
	storage     *Storage
	tempPath    string
	tempDirName string
	file        core.File
	hasher      hash.Hash
	size        int64
	finalPath   string
	buffer      []byte // Buffer to store all data
}

// NewStreamWriter creates a new streaming writer for the given path.
func (s *Storage) NewStreamWriter(ctx context.Context, path string) (*StreamWriter, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	fullPath := filepath.Join(s.rootPath, path)
	dir := filepath.Dir(fullPath)

	// Acquire file lock
	lock := s.getFileLock(fullPath)
	lock.Lock()

	// Ensure directory exists (protected by global lock)
	s.globalLock.Lock()
	err := s.fs.MkdirAll(dir, 0o755)
	if err != nil {
		s.globalLock.Unlock()
		lock.Unlock()
		return nil, fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	// Create temp file
	tempDirName, err := s.createTempDir(s.tempDir, "stream_")
	if err != nil {
		s.globalLock.Unlock()
		lock.Unlock()
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	tempFile := filepath.Join(tempDirName, "temp")

	file, err := s.fs.Create(tempFile)
	s.globalLock.Unlock()
	if err != nil {
		lock.Unlock()
		return nil, fmt.Errorf("failed to create temp file %q: %w", tempFile, err)
	}

	return &StreamWriter{
		storage:     s,
		tempPath:    tempFile,
		tempDirName: tempDirName,
		file:        file,
		hasher:      sha256.New(),
		finalPath:   fullPath,
	}, nil
}

// Write writes data to the stream writer.
func (sw *StreamWriter) Write(data []byte) (int, error) {
	// Buffer the data for later writing with checksum
	sw.buffer = append(sw.buffer, data...)
	// Note: hasher is used later in Close(), no concurrent access here
	sw.hasher.Write(data)
	sw.size += int64(len(data))
	return len(data), nil
}

// Close finalizes the stream write operation atomically.
func (sw *StreamWriter) Close() error {
	defer sw.file.Close()

	// Use the existing writeWithChecksum method to write the buffered data
	if err := sw.storage.writeWithChecksum(sw.tempPath, sw.buffer); err != nil {
		return fmt.Errorf("failed to write with checksum: %w", err)
	}

	// Atomic rename (protected by global lock)
	sw.storage.globalLock.Lock()
	err := sw.storage.fs.Rename(sw.tempPath, sw.finalPath)
	if err != nil {
		sw.storage.globalLock.Unlock()
		return fmt.Errorf("failed to rename temp file to %q: %w", sw.finalPath, err)
	}

	// Clean up temp directory
	if exists, _ := sw.storage.fs.Exists(sw.tempDirName); exists {
		_ = sw.storage.fs.Remove(sw.tempDirName) // Ignore errors in cleanup
	}
	sw.storage.globalLock.Unlock()

	// Release file lock
	lock := sw.storage.getFileLock(sw.finalPath)
	lock.Unlock()

	return nil
}

// Size returns the current size of the written data.
func (sw *StreamWriter) Size() int64 {
	return sw.size
}
