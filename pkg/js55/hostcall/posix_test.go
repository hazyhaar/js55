// SPDX-License-Identifier: Apache-2.0 OR MIT

package hostcall

import (
	"context"
	"errors"
	"testing"
)

type fixedPOSIX struct {
	uname string
}

func (f fixedPOSIX) Uname(context.Context) (string, error) { return f.uname, nil }
func (f fixedPOSIX) ReadFile(context.Context, string) ([]byte, error) {
	return nil, errors.New("hostcall: read not provided")
}
func (f fixedPOSIX) WriteFile(context.Context, string, []byte) error { return nil }
func (f fixedPOSIX) Close() error                                    { return nil }

var _ POSIX = fixedPOSIX{}

func TestPOSIX_UnameFixed(t *testing.T) {
	var p POSIX = fixedPOSIX{uname: "Linux testhost 6.14.0"}
	got, err := p.Uname(context.Background())
	if err != nil {
		t.Fatalf("Uname: %v", err)
	}
	if got != "Linux testhost 6.14.0" {
		t.Fatalf("Uname = %q", got)
	}
}
