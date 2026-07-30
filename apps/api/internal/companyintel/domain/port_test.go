package domain

import "testing"

func TestNormalizeCompanyName(t *testing.T) {
	cases := map[string]string{
		"Acme Corp":   "acme corp",
		"  ACME CORP": "acme corp",
		"":            "",
		"   ":         "",
	}
	for in, want := range cases {
		if got := NormalizeCompanyName(in); got != want {
			t.Errorf("NormalizeCompanyName(%q) = %q, want %q", in, got, want)
		}
	}
}
