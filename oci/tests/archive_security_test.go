//go:build integration

package ocibundle_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	ocibundle "github.com/jmgilman/go/oci"
	"github.com/jmgilman/go/oci/internal/testutil"
)

// TestExtractionBlocksOWASPArchives ensures extraction rejects malicious patterns.
func TestExtractionBlocksOWASPArchives(t *testing.T) {
	t.Parallel()
	gen, err := testutil.NewMaliciousArchiveGenerator()
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}
	t.Cleanup(func() { _ = gen.Close() })

	archiver := ocibundle.NewTarGzArchiver()

	// 1) Path traversal
	pt := filepath.Join(t.TempDir(), "pt.tar.gz")
	if err := gen.GeneratePathTraversalArchive(pt); err != nil {
		t.Fatalf("generate path traversal: %v", err)
	}
	ptBytes, _ := os.ReadFile(pt)
	if err := archiver.Extract(context.Background(), bytes.NewReader(ptBytes), t.TempDir(), ocibundle.DefaultExtractOptions); err == nil {
		t.Fatalf("expected path traversal archive to be rejected")
	}

	// 2) Symlink bomb
	sb := filepath.Join(t.TempDir(), "sb.tar.gz")
	if err := gen.GenerateSymlinkBomb(sb); err != nil {
		t.Fatalf("generate symlink bomb: %v", err)
	}
	sbBytes, _ := os.ReadFile(sb)
	if err := archiver.Extract(context.Background(), bytes.NewReader(sbBytes), t.TempDir(), ocibundle.DefaultExtractOptions); err == nil {
		t.Fatalf("expected symlink bomb archive to be rejected")
	}
}
