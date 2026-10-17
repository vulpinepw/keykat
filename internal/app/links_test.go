package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLink(t *testing.T) {
	page := filepath.Join(t.TempDir(), "page one.html")
	if err := os.WriteFile(page, []byte("<p>hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"https://example.com/a?b=c#d": "https://example.com/a?b=c#d",
		"HTTP://Example.com":          "http://Example.com",
		"file:///tmp/x.html":          "file:///tmp/x.html",
	} {
		if got, err := ParseLink(in); err != nil || got != want {
			t.Errorf("ParseLink(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	got, err := ParseLink(page)
	if err != nil || !strings.HasPrefix(got, "file:///") || !strings.HasSuffix(got, "/page%20one.html") {
		t.Errorf("ParseLink(file path) = %q, %v", got, err)
	}
	for _, bad := range []string{
		"", "javascript:alert(1)", "data:text/html,<script>", "chrome://browser/content/browser.xhtml",
		"about:config", "https://", "http:///nohost", filepath.Join(t.TempDir(), "missing.html"), t.TempDir(),
		"https://example.com/\nX: y",
	} {
		if got, err := ParseLink(bad); err == nil {
			t.Errorf("ParseLink(%q) accepted as %q", bad, got)
		}
	}
}

func TestLinksFromCommandLine(t *testing.T) {
	if links, err := LinksFromCommandLine("", nil, 2); err != nil || links != nil {
		t.Errorf("no link: %v %v", links, err)
	}
	if links, err := LinksFromCommandLine("https://example.com", nil, 0); err != nil || len(links) != 1 {
		t.Errorf("--url: %v %v", links, err)
	}
	if links, err := LinksFromCommandLine("", []string{"https://a.example", "https://b.example"}, 0); err != nil || len(links) != 2 {
		t.Errorf("arguments: %v %v", links, err)
	}
	for name, c := range map[string]struct {
		url    string
		args   []string
		others int
	}{
		"a link with other options":        {"https://example.com", nil, 1},
		"arguments after --url":            {"https://example.com", []string{"--setup-core"}, 0},
		"an argument link with an option":  {"", []string{"https://example.com"}, 1},
		"a script address":                 {"javascript:alert(1)", nil, 0},
		"a link that is not a web address": {"", []string{"ms-settings:privacy"}, 0},
	} {
		if links, err := LinksFromCommandLine(c.url, c.args, c.others); err == nil {
			t.Errorf("%s accepted: %v", name, links)
		}
	}
}

func TestSettingsOpenLinks(t *testing.T) {
	s := DefaultSettings()
	for _, v := range []string{"auto", "ask", "p0123456789ab"} {
		s.OpenLinks = v
		if err := s.Validate(); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
	for _, v := range []string{"", "always", "../p0123456789ab"} {
		s.OpenLinks = v
		if err := s.Validate(); err == nil {
			t.Errorf("%q accepted", v)
		}
	}
	store := LoadSettings(t.TempDir())
	if store.Get().OpenLinks != "auto" {
		t.Errorf("default is %q", store.Get().OpenLinks)
	}
}
