package http

import "testing"

func TestDownloadDisposition(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CV_Ada_Lovelace.pdf", `attachment; filename="CV_Ada_Lovelace.pdf"`},
		{"resume.pdf", `attachment; filename="resume.pdf"`},
		{
			"CV_Ада_Лавлейс.pdf",
			`attachment; filename="CV__.pdf"; filename*=UTF-8''CV_%D0%90%D0%B4%D0%B0_%D0%9B%D0%B0%D0%B2%D0%BB%D0%B5%D0%B9%D1%81.pdf`,
		},
		{
			"Ада.pdf",
			`attachment; filename="document.pdf"; filename*=UTF-8''%D0%90%D0%B4%D0%B0.pdf`,
		},
	}
	for _, c := range cases {
		if got := downloadDisposition(c.in); got != c.want {
			t.Errorf("downloadDisposition(%q) =\n  %q\nwant\n  %q", c.in, got, c.want)
		}
	}
}
