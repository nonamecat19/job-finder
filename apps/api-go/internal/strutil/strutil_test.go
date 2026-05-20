package strutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncate_ASCII(t *testing.T) {
	if got := Truncate("hello world", 5); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	if got := Truncate("hi", 5); got != "hi" {
		t.Errorf("got %q, want %q (no-op when under limit)", got, "hi")
	}
}

func TestTruncate_NeverSplitsUTF8Sequence(t *testing.T) {
	// "Довідка" (Ukrainian, all multi-byte Cyrillic) mirrors real djinni/dou
	// scrape content. A byte-slice s[:4] would land mid-codepoint.
	s := "Довідка про роботу"
	for n := 1; n <= utf8.RuneCountInString(s)+2; n++ {
		got := Truncate(s, n)
		if !utf8.ValidString(got) {
			t.Fatalf("Truncate(%q, %d) produced invalid UTF-8: %q", s, n, got)
		}
	}
}

func TestTruncate_RuneCountRespected(t *testing.T) {
	s := "Довідка" // 7 runes, 13 bytes
	got := Truncate(s, 3)
	if utf8.RuneCountInString(got) != 3 {
		t.Errorf("expected 3 runes, got %d (%q)", utf8.RuneCountInString(got), got)
	}
	if got != "Дов" {
		t.Errorf("got %q, want %q", got, "Дов")
	}
}

func TestTruncate_ZeroAndNegative(t *testing.T) {
	if got := Truncate("hello", 0); got != "" {
		t.Errorf("Truncate(_, 0) = %q, want empty", got)
	}
	if got := Truncate("hello", -1); got != "" {
		t.Errorf("Truncate(_, -1) = %q, want empty", got)
	}
}
