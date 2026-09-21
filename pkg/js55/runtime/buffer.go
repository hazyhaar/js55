// SPDX-License-Identifier: Apache-2.0 OR MIT

package runtime

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// Buffer wraps a Go byte slice with zero-copy semantics for JavaScript.
type Buffer struct {
	data []byte
}

func NewBuffer(size int) *Buffer {
	return &Buffer{data: make([]byte, size)}
}

func FromBytes(b []byte) *Buffer {
	return &Buffer{data: b}
}

func FromString(s string, encoding string) (*Buffer, error) {
	switch encoding {
	case "hex":
		decoded, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid hex string: %w", err)
		}
		return &Buffer{data: decoded}, nil
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 string: %w", err)
		}
		return &Buffer{data: decoded}, nil
	case "utf8", "utf-8", "":
		return &Buffer{data: []byte(s)}, nil
	default:
		return nil, errors.New("unsupported encoding: " + encoding)
	}
}

func (b *Buffer) Bytes() []byte {
	return b.data
}

func (b *Buffer) Length() int {
	return len(b.data)
}

func (b *Buffer) ToString(encoding string) string {
	switch encoding {
	case "hex":
		return hex.EncodeToString(b.data)
	case "base64":
		return base64.StdEncoding.EncodeToString(b.data)
	default:
		return string(b.data)
	}
}
