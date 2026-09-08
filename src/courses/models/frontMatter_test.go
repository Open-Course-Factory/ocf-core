package models

import "testing"

func TestParseFrontMatter(t *testing.T) {
	type meta struct {
		Title string `yaml:"title"`
	}
	cases := []struct {
		name, in, wantTitle, wantBody string
	}{
		{"block then body", "---\ntitle: A\n---\nbody\n", "A", "body\n"},
		{"leading blank lines", "\n\n---\ntitle: A\n---\nbody", "A", "body"},
		{"no front matter", "# hello\n---\ntitle: A\n---\n", "", "# hello\n---\ntitle: A\n---\n"},
		{"empty front matter", "---\n---\nbody", "", "body"},
		{"delimiter inside body", "---\ntitle: A\n---\nx\n---\ny", "A", "x\n---\ny"},
		{"crlf", "---\r\ntitle: A\r\n---\r\nbody\r\n", "A", "body\r\n"},
		{"unterminated block", "---\ntitle: A\nbody", "", "---\ntitle: A\nbody"},
		{"closing delimiter at eof", "---\ntitle: A\n---", "A", ""},
		{"empty input", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var m meta
			body, err := parseFrontMatter([]byte(c.in), &m)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Title != c.wantTitle || string(body) != c.wantBody {
				t.Fatalf("got title %q body %q, want %q %q", m.Title, body, c.wantTitle, c.wantBody)
			}
		})
	}
}
