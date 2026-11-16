package bitstream

import "testing"

func TestWindows1252Punctuation(t *testing.T) {
	if got := DecodeCodepage([]byte{0x97, 0x20, 0x80}, 30); got != "— €" {
		t.Fatalf("ANSI_1252 punctuation decoded as %q", got)
	}
}
