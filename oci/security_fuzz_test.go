package ocibundle

import "testing"

// FuzzSizeValidator exercises SizeValidator against arbitrary sizes to ensure stability.
func FuzzSizeValidator(f *testing.F) {
	// Seed typical boundaries
	f.Add(int64(0), int64(0), int64(0))                 // no limits, zero size
	f.Add(int64(1024), int64(10*1024), int64(2048))     // file > per-file limit
	f.Add(int64(1024*1024), int64(1024*1024), int64(1)) // tiny file

	f.Fuzz(func(t *testing.T, maxFile, maxTotal, fileSize int64) {
		v := NewSizeValidator(abs64(maxFile%(1<<30)), abs64(maxTotal%(1<<31)))
		info := FileInfo{Name: "f", Size: abs64(fileSize % (1 << 31)), Mode: 0o644}
		_ = v.ValidateFile(info)
		_ = v.ValidateArchive(ArchiveStats{TotalFiles: 1, TotalSize: info.Size})
	})
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
