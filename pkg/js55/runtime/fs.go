// SPDX-License-Identifier: BUSL-1.1

package runtime

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SandboxedFS restricts all filesystem operations to a designated RootDir.
type SandboxedFS struct {
	RootDir  string
	ReadOnly bool
}

func NewSandboxedFS(rootDir string, readOnly bool) (*SandboxedFS, error) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	return &SandboxedFS{
		RootDir:  abs,
		ReadOnly: readOnly,
	}, nil
}

func (s *SandboxedFS) resolvePath(path string) (string, error) {
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("fs: access denied (null byte in path)")
	}

	// Also protect against URL encoded relative paths (%2e%2e%2f)
	unescaped, err := url.PathUnescape(path)
	if err == nil {
		path = unescaped
	}

	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		clean = filepath.Join(s.RootDir, clean)
	}

	rootResolved, err := filepath.EvalSymlinks(s.RootDir)
	if err != nil {
		rootResolved = s.RootDir
	}

	// Find the longest existing ancestor path to evaluate symlinks recursively
	curr := clean
	var unexistingParts []string
	for {
		_, err := os.Lstat(curr)
		if err == nil {
			// curr exists
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			// Reached root
			break
		}
		unexistingParts = append([]string{filepath.Base(curr)}, unexistingParts...)
		curr = parent
	}

	// Resolve symlinks of the existing ancestor
	resolvedAncestor, err := filepath.EvalSymlinks(curr)
	if err != nil {
		return "", fmt.Errorf("fs: failed to resolve ancestor symlinks: %w", err)
	}

	// Check that the resolved ancestor is strictly inside rootResolved
	rel, err := filepath.Rel(rootResolved, resolvedAncestor)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("fs: access denied (symlink escape outside sandbox): %s", path)
	}

	// Reconstruct the full path
	finalPath := resolvedAncestor
	for _, part := range unexistingParts {
		finalPath = filepath.Join(finalPath, part)
	}

	// Double check final relative path
	finalRel, err := filepath.Rel(rootResolved, finalPath)
	if err != nil || strings.HasPrefix(finalRel, "..") || finalRel == ".." {
		return "", fmt.Errorf("fs: access denied (path outside sandbox): %s", path)
	}

	return finalPath, nil
}

func (s *SandboxedFS) ReadFile(path string) ([]byte, error) {
	target, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}

func (s *SandboxedFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	if s.ReadOnly {
		return fmt.Errorf("fs: write denied (filesystem is mounted read-only)")
	}
	target, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, perm)
}

func (s *SandboxedFS) Stat(path string) (os.FileInfo, error) {
	target, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.Stat(target)
}
