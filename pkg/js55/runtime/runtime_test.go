// SPDX-License-Identifier: BUSL-1.1

package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxedFS_Enforcement(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewSandboxedFS(tmpDir, false)
	if err != nil {
		t.Fatalf("NewSandboxedFS: %v", err)
	}

	testFile := filepath.Join(tmpDir, "hello.txt")
	err = fs.WriteFile("hello.txt", []byte("world"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	content, err := fs.ReadFile("hello.txt")
	if err != nil || string(content) != "world" {
		t.Fatalf("ReadFile failed: got %q, want %q", string(content), "world")
	}

	// Escape attack test
	_, err = fs.ReadFile("../../etc/passwd")
	if err == nil {
		t.Fatalf("expected sandbox traversal error, got nil")
	}

	_ = os.Remove(testFile)
}
