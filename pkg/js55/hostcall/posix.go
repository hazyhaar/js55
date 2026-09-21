// SPDX-License-Identifier: BUSL-1.1

package hostcall

import "context"

type POSIX interface {
	Uname(ctx context.Context) (string, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	Close() error
}
