package main

import (
	"strings"
	"testing"
)

func TestStripJS(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"line comment alone", "a();\n  // note\nb();\n", "a();\nb();\n"},
		{"trailing comment", "a(); // note\nb();\n", "a();\nb();\n"},
		{"block comment alone", "/* header\n * more\n */\nconst x = 1;\n", "const x = 1;\n"},
		{"inline block comment", "f(a, /* why */ b);\n", "f(a, b);\n"},
		{"multiline block keeps a newline for ASI", "let a = 1 /*\n*/ let b = 2\n", "let a = 1\n let b = 2\n"},
		{"url in string", `const u = "https://example.com/*x*/"; // c` + "\n", `const u = "https://example.com/*x*/";` + "\n"},
		{"regex with slashes", "if (/^\\/\\/.*/.test(s)) x(); // c\n", "if (/^\\/\\/.*/.test(s)) x();\n"},
		{"regex with class", "s.replace(/[/*]/g, \"\"); /* c */\n", "s.replace(/[/*]/g, \"\");\n"},
		{"division is not a regex", "const r = a / b / c; // half\n", "const r = a / b / c;\n"},
		{"division after paren", "const r = (a + b) / 2 / 3;\n", "const r = (a + b) / 2 / 3;\n"},
		{"regex after return", "return /a\\/b/.test(x); // c\n", "return /a\\/b/.test(x);\n"},
		{"template keeps text", "const t = `// not a comment ${a /* c */ + b} /* text */`;\n", "const t = `// not a comment ${a + b} /* text */`;\n"},
		{"nested template", "x = `a ${`b ${c // d\n}`} e`;\n", "x = `a ${`b ${c\n}`} e`;\n"},
		{"object in substitution", "x = `${ {a: 1}.a /* c */ }`;\n", "x = `${ {a: 1}.a }`;\n"},
		{"escaped quote", `s = 'it\'s // fine'; // c` + "\n", `s = 'it\'s // fine';` + "\n"},
		{"arrow then regex", "const f = s => /x\\/y/.test(s);\n", "const f = s => /x\\/y/.test(s);\n"},
	}
	for _, c := range cases {
		if got := StripJS(c.in); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestStripCSSAndMarkup(t *testing.T) {
	css := "/* header */\n.a {\n  content: \"/* kept */\"; /* note */\n  color: red;\n}\n"
	if got, want := StripCSS(css), ".a {\n  content: \"/* kept */\";\n  color: red;\n}\n"; got != want {
		t.Errorf("css:\n got %q\nwant %q", got, want)
	}
	html := "<!doctype html>\n<!-- note -->\n<p>Hi <!-- x --> there</p>\n"
	if got, want := StripMarkup(html), "<!doctype html>\n<p>Hi there</p>\n"; got != want {
		t.Errorf("markup:\n got %q\nwant %q", got, want)
	}
}

func TestStripHashLinesAndCfg(t *testing.T) {
	ftl := "# License\n# more\n\n## Section\nmsg = Hello\n    .title = # not a comment\n"
	if got, want := StripHashLines(ftl), "msg = Hello\n    .title = # not a comment\n"; got != want {
		t.Errorf("ftl:\n got %q\nwant %q", got, want)
	}
	cfg, _ := Strip("keycat.cfg", []byte("// first line\n// more\ntry {\n  go(); // c\n} catch (e) {}\n"))
	if !strings.HasPrefix(string(cfg), "\ntry {") || strings.Contains(string(cfg), "//") {
		t.Errorf("cfg: %q", cfg)
	}
}
