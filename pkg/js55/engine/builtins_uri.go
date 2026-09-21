// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"net/url"
)

// BuiltinEncodeURI encodes a URI string following ECMAScript encodeURI specification.
func BuiltinEncodeURI(s string) string {
	return url.PathEscape(s)
}

// BuiltinDecodeURI decodes a URI string.
func BuiltinDecodeURI(s string) (string, error) {
	return url.PathUnescape(s)
}

// BuiltinEncodeURIComponent encodes URI component.
func BuiltinEncodeURIComponent(s string) string {
	return url.QueryEscape(s)
}

// BuiltinDecodeURIComponent decodes URI component.
func BuiltinDecodeURIComponent(s string) (string, error) {
	return url.QueryUnescape(s)
}
