package model

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// CleanText makes a string a device reported safe to store and show.
//
// Agents sometimes wrap text in binary framing: HP PML prefixes octet strings
// with a type header (01 15 on older firmware, FD E8 on newer), and many
// agents pad with NULs. Left in, the header is invalid UTF-8 and renders as
// "��" in front of the model name. CleanText drops a leading run of control or
// invalid bytes, drops any other invalid UTF-8 and control characters, and
// trims surrounding space. Text that is already clean is returned unchanged.
func CleanText(s string) string {
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if (r == utf8.RuneError && size == 1) || unicode.IsControl(r) {
			s = s[size:]
			continue
		}
		break
	}
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}
