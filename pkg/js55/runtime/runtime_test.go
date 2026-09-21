// SPDX-License-Identifier: Apache-2.0 OR MIT

package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuffer_ZeroCopyAndEncodings(t *testing.T) {
	raw := []byte("Hello, js55!")
	buf := FromBytes(raw)

	if buf.Length() != 12 {
		t.Fatalf("length mismatch: got %d, want 12", buf.Length())
	}

	hexStr := buf.ToString("hex")
	b64Str := buf.ToString("base64")

	fromHex, err := FromString(hexStr, "hex")
	if err != nil || fromHex.ToString("utf8") != "Hello, js55!" {
		t.Fatalf("hex roundtrip failed: %v", err)
	}

	fromB64, err := FromString(b64Str, "base64")
	if err != nil || fromB64.ToString("utf8") != "Hello, js55!" {
		t.Fatalf("base64 roundtrip failed: %v", err)
	}
}

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
