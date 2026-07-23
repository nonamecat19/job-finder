package application

import "testing"

func TestSanitize(t *testing.T) {
	got := sanitize("Acme Corp / Special Chars!! — Senior Engineer")
	want := "acme_corp_special_chars_senior_engineer"
	if got != want {
		t.Errorf("sanitize() = %q, want %q", got, want)
	}
}
