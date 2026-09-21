// SPDX-License-Identifier: Apache-2.0 OR MIT
package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxedFS_Attack(t *testing.T) {
	// Create a temporary directory acting as our sandbox root
	root := t.TempDir()

	// Create a file outside the sandbox
	outsideDir := t.TempDir()
	secretFile := filepath.Join(outsideDir, "secret.txt")
	os.WriteFile(secretFile, []byte("super secret"), 0644)

	fs, err := NewSandboxedFS(root, false)
	if err != nil {
		t.Fatalf("failed to create SandboxedFS: %v", err)
	}

	// Create a symlink inside the sandbox pointing outside
	symlinkPath := filepath.Join(root, "link_to_secret")
	err = os.Symlink(secretFile, symlinkPath)
	if err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Create a symlink directory pointing outside
	linkDir := filepath.Join(root, "linkdir")
	err = os.Symlink(outsideDir, linkDir)
	if err != nil {
		t.Fatalf("failed to create symlink dir: %v", err)
	}

	attacks := []string{
		"../secret.txt",
		"..//../secret.txt",
		filepath.Join("..", outsideDir, "secret.txt"),
		"link_to_secret",
		"link_to_secret/../../../etc/passwd",
		"linkdir/new_file.txt",
		"linkdir/pwned.txt",
		"file\x00name.txt", // null byte
		"....//something",
		"%2e%2e%2fsecret.txt", // url encoded if it gets passed as is
	}

	for _, attack := range attacks {
		_, err := fs.ReadFile(attack)
		if err == nil {
			t.Errorf("Attack succeeded! Was able to read file with payload: %q", attack)
		}

		err = fs.WriteFile(attack, []byte("hacked"), 0644)
		if err == nil {
			t.Errorf("Attack succeeded! Was able to write file with payload: %q", attack)
		}
	}
}
