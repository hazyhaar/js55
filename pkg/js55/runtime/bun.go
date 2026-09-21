// SPDX-License-Identifier: Apache-2.0 OR MIT

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
)

// BunCompat provides direct emulation of Bun's fast-path native APIs.
type BunCompat struct {
	fs *SandboxedFS
}

func NewBunCompat(fs *SandboxedFS) *BunCompat {
	return &BunCompat{fs: fs}
}

// Bun.file(path) -> returns a fast-path BunFile descriptor
type BunFile struct {
	path string
	fs   *SandboxedFS
}

func (b *BunCompat) File(path string) *BunFile {
	return &BunFile{path: path, fs: b.fs}
}

func (bf *BunFile) Text() (string, error) {
	data, err := bf.fs.ReadFile(bf.path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (bf *BunFile) ArrayBuffer() ([]byte, error) {
	return bf.fs.ReadFile(bf.path)
}

func (bf *BunFile) Size() (int64, error) {
	fi, err := bf.fs.Stat(bf.path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// Bun.write(destination, data) -> fast file writer
func (b *BunCompat) Write(dest string, data []byte) (int, error) {
	err := b.fs.WriteFile(dest, data, 0644)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// Bun.hash(data) -> fast-path hashing (SHA-256 / AVX2 accelerated)
func (b *BunCompat) Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Bun.serve({ port, fetch }) -> fast HTTP server powered directly by Go Netpoller
func (b *BunCompat) Serve(addr string, handler func(w http.ResponseWriter, r *http.Request)) (*http.Server, error) {
	server := &http.Server{
		Addr:    addr,
		Handler: http.HandlerFunc(handler),
	}
	go func() {
		_ = server.ListenAndServe()
	}()
	return server, nil
}

// Bun.env -> process environment map
func (b *BunCompat) Env() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		var k, v string
		_, _ = fmt.Sscanf(e, "%s=%s", &k, &v)
		if k != "" {
			env[k] = v
		}
	}
	return env
}
