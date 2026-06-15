package strutil

import (
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long string",
			input:    "hello world",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "zero max length",
			input:    "hello",
			maxLen:   0,
			expected: "",
		},
		{
			name:     "negative max length",
			input:    "hello",
			maxLen:   -1,
			expected: "",
		},
		{
			name:     "single character",
			input:    "a",
			maxLen:   1,
			expected: "a",
		},
		{
			name:     "truncate at 1",
			input:    "hello",
			maxLen:   1,
			expected: "h",
		},
		{
			name:     "truncate at 2",
			input:    "hello",
			maxLen:   2,
			expected: "he",
		},
		{
			name:     "truncate at 3",
			input:    "hello",
			maxLen:   3,
			expected: "hel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("for input %q with maxLen %d, expected %q, got %q", tt.input, tt.maxLen, tt.expected, result)
			}
		})
	}
}

func TestTruncateWithUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "unicode characters",
			input:    "你好世界",
			maxLen:   10,
			expected: "你好世界",
		},
		{
			name:     "unicode truncation",
			input:    "你好世界",
			maxLen:   2,
			expected: "你好",
		},
		{
			name:     "emoji",
			input:    "👋🌍🎉",
			maxLen:   10,
			expected: "👋🌍🎉",
		},
		{
			name:     "mixed unicode and ascii",
			input:    "hello 你好 world",
			maxLen:   8,
			expected: "hello 你好",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("for input %q with maxLen %d, expected %q, got %q", tt.input, tt.maxLen, tt.expected, result)
			}
		})
	}
}

func TestTruncatePerformance(t *testing.T) {
	longString := strings.Repeat("a", 10000)
	result := Truncate(longString, 100)

	runes := []rune(result)
	if len(runes) != 100 {
		t.Errorf("expected 100 runes, got %d", len(runes))
	}
}

func TestTruncateEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "whitespace only",
			input:    "   ",
			maxLen:   2,
			expected: "  ",
		},
		{
			name:     "newlines",
			input:    "line1\nline2",
			maxLen:   5,
			expected: "line1",
		},
		{
			name:     "tabs",
			input:    "col1\tcol2",
			maxLen:   5,
			expected: "col1\t",
		},
		{
			name:     "special characters",
			input:    "hello@world.com",
			maxLen:   10,
			expected: "hello@worl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("for input %q with maxLen %d, expected %q, got %q", tt.input, tt.maxLen, tt.expected, result)
			}
		})
	}
}

func TestTruncateMaxLengthOne(t *testing.T) {
	input := "hello"
	result := Truncate(input, 1)

	if result != "h" {
		t.Errorf("expected 'h', got %q", result)
	}
}

func TestTruncateMaxLengthTwo(t *testing.T) {
	input := "hello"
	result := Truncate(input, 2)

	if result != "he" {
		t.Errorf("expected 'he', got %q", result)
	}
}

func TestTruncateWithNegativeMaxLength(t *testing.T) {
	result := Truncate("hello", -1)

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestTruncateLongString(t *testing.T) {
	input := strings.Repeat("a", 1000)
	result := Truncate(input, 100)

	runes := []rune(result)
	if len(runes) != 100 {
		t.Errorf("expected 100 runes, got %d", len(runes))
	}
}

func TestTruncateWithEmoji(t *testing.T) {
	input := "👋🌍🎉"
	result := Truncate(input, 2)

	if result != "👋🌍" {
		t.Errorf("expected '👋🌍', got %q", result)
	}
}

func TestTruncatePreserveContent(t *testing.T) {
	input := "abcdefghij"
	result := Truncate(input, 5)

	if !strings.HasPrefix(result, "abcde") {
		t.Errorf("expected result to start with 'abcde', got %q", result)
	}
}

func TestTruncateMultipleCalls(t *testing.T) {
	input := "hello world"

	result1 := Truncate(input, 5)
	result2 := Truncate(input, 10)
	result3 := Truncate(input, 20)

	if result1 != "hello" {
		t.Errorf("expected 'hello', got %q", result1)
	}
	if result2 != "hello worl" {
		t.Errorf("expected 'hello worl', got %q", result2)
	}
	if result3 != "hello world" {
		t.Errorf("expected 'hello world', got %q", result3)
	}
}

func TestTruncateWithSpecialChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "html entities",
			input:    "&amp; &lt; &gt;",
			maxLen:   5,
			expected: "&amp;",
		},
		{
			name:     "quotes",
			input:    "\"hello\" 'world'",
			maxLen:   8,
			expected: "\"hello\" ",
		},
		{
			name:     "backslashes",
			input:    "path\\to\\file",
			maxLen:   8,
			expected: "path\\to\\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("for input %q with maxLen %d, expected %q, got %q", tt.input, tt.maxLen, tt.expected, result)
			}
		})
	}
}

func TestTruncateWithNewlines(t *testing.T) {
	input := "line1\nline2\nline3"
	result := Truncate(input, 10)

	if !strings.Contains(result, "line1") {
		t.Error("result should contain 'line1'")
	}
}

func TestTruncateWithTabs(t *testing.T) {
	input := "col1\tcol2\tcol3"
	result := Truncate(input, 10)

	if !strings.Contains(result, "col1") {
		t.Error("result should contain 'col1'")
	}
}

func TestTruncateMaxLength(t *testing.T) {
	input := "hello"
	result := Truncate(input, 1000000)

	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateWithZeroMaxLength(t *testing.T) {
	result := Truncate("hello", 0)

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}
