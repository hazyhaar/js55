// SPDX-License-Identifier: Apache-2.0 OR MIT

package isolate

import (
	"context"
	"errors"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/hostcall"
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

var _ hostcall.POSIX = fixedPOSIX{}

func TestIsolate_POSIXOptional(t *testing.T) {
	iso, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := iso.Eval(context.Background(), `1+1`)
	if err != nil {
		t.Fatalf("Eval sans POSIX: %v", err)
	}
	if got := v.ToInt(); got != 2 {
		t.Fatalf("1+1 = %d", got)
	}
	if iso.POSIX() != nil {
		t.Fatal("POSIX absent doit rester nil")
	}

	const want = "Linux testhost 6.14.0-fixed"
	iso2, err := New(Config{POSIX: fixedPOSIX{uname: want}})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := iso2.Eval(context.Background(), `1+1`)
	if err != nil {
		t.Fatalf("Eval avec POSIX: %v", err)
	}
	if got := v2.ToInt(); got != 2 {
		t.Fatalf("1+1 = %d", got)
	}
	p := iso2.POSIX()
	if p == nil {
		t.Fatal("POSIX fourni doit être exposé")
	}
	got, err := p.Uname(context.Background())
	if err != nil {
		t.Fatalf("POSIX().Uname: %v", err)
	}
	if got != want {
		t.Fatalf("POSIX().Uname = %q, attendu %q", got, want)
	}
}
