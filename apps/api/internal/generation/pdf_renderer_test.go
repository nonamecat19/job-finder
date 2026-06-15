package generation

import (
	"reflect"
	"testing"
)

func TestSplitParagraphs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Hello.\n\nWorld.", []string{"Hello.", "World."}},
		{"Hello.\nWorld.", []string{"Hello.", "World."}},
		{"  Hello.  \n\n  World.  ", []string{"Hello.", "World."}},
		{"OnlyOne", []string{"OnlyOne"}},
		{"\n\n", nil},
	}
	for _, c := range cases {
		got := splitParagraphs(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitParagraphs(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestSanitize(t *testing.T) {
	got := sanitize("Acme Corp / Special Chars!! — Senior Engineer")
	want := "acme_corp_special_chars_senior_engineer"
	if got != want {
		t.Errorf("sanitize() = %q, want %q", got, want)
	}
}
