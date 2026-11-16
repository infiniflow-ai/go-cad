package drawing

import "testing"

func TestStripMTextParagraphProperties(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"\\pxqc;1", "1"},
		{"\\pxi1,ql;TEXT", "TEXT"},
		{"\\pxsm0.7,qc;TEXT", "TEXT"},
		{"\\pxb0.25,sm0.7,qc;X", "X"},
		{"before\\P\\pxqc;after", "before\nafter"},
	} {
		if got := StripMTextFormat(tc.raw); got != tc.want {
			t.Errorf("StripMTextFormat(%q)=%q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestStripMTextUnicodeEscapes(t *testing.T) {
	if got := StripMTextFormat(`Albero \U+230025 h7`); got != "Albero ⌀25 h7" {
		t.Fatalf("Unicode escape remained in text: %q", got)
	}
}
