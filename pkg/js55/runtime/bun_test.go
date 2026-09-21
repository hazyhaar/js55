// SPDX-License-Identifier: Apache-2.0 OR MIT

package runtime

import (
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestBunCompat_FileAndWrite(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewSandboxedFS(tmpDir, false)
	if err != nil {
		t.Fatalf("NewSandboxedFS failed: %v", err)
	}

	bun := NewBunCompat(fs)

	// Test Bun.write
	n, err := bun.Write("bun_test.txt", []byte("Hello from BunCompat in Go!"))
	if err != nil || n != 27 {
		t.Fatalf("Bun.write failed: n = %d, err = %v", n, err)
	}

	// Test Bun.file
	f := bun.File("bun_test.txt")
	txt, err := f.Text()
	if err != nil || txt != "Hello from BunCompat in Go!" {
		t.Fatalf("BunFile.Text failed: got %q, want %q", txt, "Hello from BunCompat in Go!")
	}

	size, err := f.Size()
	if err != nil || size != 27 {
		t.Fatalf("BunFile.Size failed: got %d, want 27", size)
	}

	hash := bun.Hash([]byte("Hello from BunCompat in Go!"))
	if len(hash) != 64 {
		t.Fatalf("Bun.hash failed: got %q", hash)
	}

	_ = filepath.Join(tmpDir, "bun_test.txt")
}

func TestBunCompat_Serve(t *testing.T) {
	tmpDir := t.TempDir()
	fs, _ := NewSandboxedFS(tmpDir, false)
	bun := NewBunCompat(fs)

	server, err := bun.Serve("127.0.0.1:18999", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Bun.serve OK"))
	})
	if err != nil {
		t.Fatalf("Bun.serve: %v", err)
	}
	defer func() { _ = server.Close() }()

	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:18999/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Bun.serve OK" {
		t.Fatalf("got %q, want %q", string(body), "Bun.serve OK")
	}
}
