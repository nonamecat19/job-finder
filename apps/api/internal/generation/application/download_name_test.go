package application

import "testing"

func TestDownloadNameSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Ada Lovelace", "Ada_Lovelace"},
		{"Ada  Lovelace-King", "Ada_Lovelace_King"},
		{"  Ada Lovelace  ", "Ada_Lovelace"},
		{"Ada", "Ada"},
		{"", ""},
		{"   ", ""},
		{"Ада Лавлейс", "Ада_Лавлейс"},
		{"José Ángel Díaz", "José_Ángel_Díaz"},
		{"O'Neill", "O_Neill"},
	}
	for _, c := range cases {
		if got := downloadNameSlug(c.in); got != c.want {
			t.Errorf("downloadNameSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
