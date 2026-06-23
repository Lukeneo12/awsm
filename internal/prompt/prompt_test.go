package prompt

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirm_core(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"lower y", "y\n", true},
		{"word yes", "yes\n", true},
		{"upper Y", "Y\n", true},
		{"upper YES", "YES\n", true},
		{"y padded with spaces", "  y  \n", true},
		{"empty is no", "\n", false},
		{"n is no", "n\n", false},
		{"word no", "no\n", false},
		{"garbage is no", "maybe\n", false},
		{"eof without newline", "y", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := confirm(strings.NewReader(tc.input), &out, "Save?")
			if err != nil {
				t.Fatalf("confirm error: %v", err)
			}
			if got != tc.want {
				t.Errorf("confirm(%q) = %v, want %v", tc.input, got, tc.want)
			}
			if !strings.Contains(out.String(), "Save? [y/N]: ") {
				t.Errorf("prompt not written to w: %q", out.String())
			}
		})
	}
}
