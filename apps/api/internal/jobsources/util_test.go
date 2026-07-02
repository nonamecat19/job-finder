package jobsources

import (
	"testing"

	"github.com/job-finder/api/internal/dto"
)

func TestPtr(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "string pointer",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "int pointer",
			input:    42,
			expected: 42,
		},
		{
			name:     "bool pointer",
			input:    true,
			expected: true,
		},
		{
			name:     "float pointer",
			input:    3.14,
			expected: 3.14,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch v := tt.input.(type) {
			case string:
				result := Ptr(v)
				if *result != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, *result)
				}
			case int:
				result := Ptr(v)
				if *result != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, *result)
				}
			case bool:
				result := Ptr(v)
				if *result != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, *result)
				}
			case float64:
				result := Ptr(v)
				if *result != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, *result)
				}
			}
		})
	}
}

func TestNilIfEmpty(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *string
	}{
		{
			name:     "non-empty string",
			input:    "hello",
			expected: stringPtr("hello"),
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: stringPtr("   "),
		},
		{
			name:     "single character",
			input:    "a",
			expected: stringPtr("a"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NilIfEmpty(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %q", *result)
				}
			} else {
				if result == nil {
					t.Errorf("expected %q, got nil", *tt.expected)
				} else if *result != *tt.expected {
					t.Errorf("expected %q, got %q", *tt.expected, *result)
				}
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestStringOr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		expected string
	}{
		{
			name:     "non-empty string",
			input:    "hello",
			fallback: "default",
			expected: "hello",
		},
		{
			name:     "empty string",
			input:    "",
			fallback: "default",
			expected: "default",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			fallback: "default",
			expected: "   ",
		},
		{
			name:     "empty fallback",
			input:    "",
			fallback: "",
			expected: "",
		},
		{
			name:     "both non-empty",
			input:    "value",
			fallback: "fallback",
			expected: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringOr(tt.input, tt.fallback)
			if result != tt.expected {
				t.Errorf("for input %q with fallback %q, expected %q, got %q", tt.input, tt.fallback, tt.expected, result)
			}
		})
	}
}

func TestPtrNilSafety(t *testing.T) {
	result := Ptr("test")
	if result == nil {
		t.Error("Ptr should not return nil")
	}
	if *result != "test" {
		t.Errorf("expected 'test', got %q", *result)
	}
}

func TestPtrZeroValue(t *testing.T) {
	strResult := Ptr("")
	if *strResult != "" {
		t.Errorf("expected empty string, got %q", *strResult)
	}

	intResult := Ptr(0)
	if *intResult != 0 {
		t.Errorf("expected 0, got %d", *intResult)
	}

	boolResult := Ptr(false)
	if *boolResult != false {
		t.Errorf("expected false, got %v", *boolResult)
	}
}

func TestNilIfEmptyWithJobType(t *testing.T) {
	str := "hello"
	result := NilIfEmpty(str)
	if result == nil {
		t.Error("expected non-nil result")
	}
	if *result != "hello" {
		t.Errorf("expected 'hello', got %q", *result)
	}
}

func TestStringOrWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback string
		expected string
	}{
		{
			name:     "special characters",
			input:    "hello@world.com",
			fallback: "default",
			expected: "hello@world.com",
		},
		{
			name:     "unicode",
			input:    "你好世界",
			fallback: "default",
			expected: "你好世界",
		},
		{
			name:     "emoji",
			input:    "👋🌍",
			fallback: "default",
			expected: "👋🌍",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringOr(tt.input, tt.fallback)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestPtrPerformance(t *testing.T) {
	for i := 0; i < 1000; i++ {
		result := Ptr(i)
		if *result != i {
			t.Errorf("expected %d, got %d", i, *result)
		}
	}
}

func TestNilIfEmptyEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *string
	}{
		{
			name:     "null string",
			input:    "null",
			expected: stringPtr("null"),
		},
		{
			name:     "nil-like string",
			input:    "nil",
			expected: stringPtr("nil"),
		},
		{
			name:     "undefined string",
			input:    "undefined",
			expected: stringPtr("undefined"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NilIfEmpty(tt.input)
			if result == nil {
				t.Errorf("expected non-nil result for %q", tt.input)
			}
			if *result != tt.input {
				t.Errorf("expected %q, got %q", tt.input, *result)
			}
		})
	}
}

func TestStringOrWithEmptyFallback(t *testing.T) {
	result := StringOr("", "")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestPtrWithComplexTypes(t *testing.T) {
	type TestStruct struct {
		Name string
		Age  int
	}

	ts := TestStruct{Name: "John", Age: 30}
	result := Ptr(ts)
	if result.Name != "John" || result.Age != 30 {
		t.Errorf("expected {John 30}, got %v", result)
	}

	slice := []int{1, 2, 3}
	sliceResult := Ptr(slice)
	if len(*sliceResult) != 3 || (*sliceResult)[0] != 1 {
		t.Errorf("expected [1 2 3], got %v", sliceResult)
	}
}

func TestNilIfEmptyWithNestedStrings(t *testing.T) {
	input := "nested: {key: value}"
	result := NilIfEmpty(input)
	if result == nil {
		t.Error("expected non-nil result")
	}
	if *result != input {
		t.Errorf("expected %q, got %q", input, *result)
	}
}

func TestStringOrPerformance(t *testing.T) {
	for i := 0; i < 1000; i++ {
		input := ""
		fallback := "fallback"
		result := StringOr(input, fallback)
		if result != fallback {
			t.Errorf("expected %q, got %q", fallback, result)
		}
	}
}

func TestPtrWithSourceKind(t *testing.T) {
	kind := dto.SourceKindAPI
	result := Ptr(kind)
	if *result != dto.SourceKindAPI {
		t.Errorf("expected %v, got %v", dto.SourceKindAPI, *result)
	}
}
