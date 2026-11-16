package drawing

import (
	"strconv"
	"strings"
	"unicode/utf16"
)

// expandUnicodeEscapes decodes CAD's four-hex-digit Unicode notation. Invalid
// escapes and escaped backslashes remain literal instead of losing text.
func expandUnicodeEscapes(s string) string {
	if !strings.Contains(s, `\U+`) {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '\\' && s[i+1] == '\\' {
			out.WriteString(s[i : i+2])
			i += 2
			continue
		}
		if i+7 <= len(s) && s[i:i+3] == `\U+` {
			value, err := strconv.ParseUint(s[i+3:i+7], 16, 16)
			if err == nil {
				r := rune(value)
				n := 7
				if r >= 0xD800 && r <= 0xDBFF && i+14 <= len(s) && s[i+7:i+10] == `\U+` {
					low, e := strconv.ParseUint(s[i+10:i+14], 16, 16)
					if e == nil && low >= 0xDC00 && low <= 0xDFFF {
						r = utf16.DecodeRune(r, rune(low))
						n = 14
					}
				}
				if !utf16.IsSurrogate(r) {
					out.WriteRune(r)
					i += n
					continue
				}
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}
