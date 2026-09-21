// SPDX-License-Identifier: BUSL-1.1

package str

import "testing"

func TestUTF16LenOfUTF8Prefix_Emoji(t *testing.T) {
	s := "a😀b"
	if UTF16LenOfUTF8Prefix(s, 0) != 0 {
		t.Fatal("zéro")
	}
	if UTF16LenOfUTF8Prefix(s, 1) != 1 {
		t.Fatal("ascii")
	}
	if UTF16LenOfUTF8Prefix(s, 5) != 3 {
		t.Fatalf("emoji 4 octets → 2 unités, got %d", UTF16LenOfUTF8Prefix(s, 5))
	}
	if UTF16LenOfUTF8Prefix(s, len(s)) != 4 {
		t.Fatalf("fin %d", UTF16LenOfUTF8Prefix(s, len(s)))
	}
}
