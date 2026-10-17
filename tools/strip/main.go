package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	src := flag.String("src", "", "source directory")
	dst := flag.String("dst", "", "destination directory (replaced)")
	flag.Parse()
	if *src == "" || *dst == "" {
		fmt.Fprintln(os.Stderr, "usage: strip -src DIR -dst DIR")
		os.Exit(2)
	}
	n, err := run(*src, *dst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "strip:", err)
		os.Exit(1)
	}
	fmt.Printf("stripped %d files from %s\n", n, *src)
}

func run(src, dst string) (int, error) {
	if err := os.RemoveAll(dst); err != nil {
		return 0, err
	}
	stripped := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out, changed := Strip(rel, data)
		if changed {
			stripped++
		}
		return os.WriteFile(target, out, 0o644)
	})
	return stripped, err
}

func Strip(name string, data []byte) ([]byte, bool) {
	lower := strings.ToLower(name)
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	switch {
	case strings.HasSuffix(lower, ".cfg"):
		return []byte("\n" + strings.TrimLeft(StripJS(text), "\n")), true
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".mjs"):
		return []byte(StripJS(text)), true
	case strings.HasSuffix(lower, ".css"):
		return []byte(StripCSS(text)), true
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".svg"), strings.HasSuffix(lower, ".xhtml"):
		return []byte(StripMarkup(text)), true
	case strings.HasSuffix(lower, ".ftl"), strings.HasSuffix(lower, ".ftl.override"),
		strings.HasSuffix(lower, ".properties"), strings.HasSuffix(lower, ".properties.override"),
		strings.HasSuffix(lower, ".manifest"), strings.HasSuffix(lower, ".txt"):
		return []byte(StripHashLines(text)), true
	}
	return data, false
}

type out struct{ b []byte }

func (o *out) byte(c byte)     { o.b = append(o.b, c) }
func (o *out) str(s string)    { o.b = append(o.b, s...) }
func (o *out) String() string  { return string(o.b) }
func (o *out) lineStart() int  { return strings.LastIndexByte(string(o.b), '\n') + 1 }
func (o *out) lineBlank() bool { return strings.TrimLeft(string(o.b[o.lineStart():]), " \t") == "" }
func (o *out) dropLineIndent() { o.b = o.b[:o.lineStart()] }
func (o *out) trimTrailing()   { o.b = []byte(strings.TrimRight(string(o.b), " \t")) }
func (o *out) lastByte() byte {
	if len(o.b) == 0 {
		return '\n'
	}
	return o.b[len(o.b)-1]
}

func skipComment(src string, start, end int, o *out) int {
	body := src[start:end]
	restOfLine := end
	for restOfLine < len(src) && (src[restOfLine] == ' ' || src[restOfLine] == '\t') {
		restOfLine++
	}
	atLineEnd := restOfLine >= len(src) || src[restOfLine] == '\n'
	if o.lineBlank() && atLineEnd {
		o.dropLineIndent()
		if restOfLine < len(src) {
			return restOfLine + 1
		}
		return restOfLine
	}
	if strings.Contains(body, "\n") {
		o.trimTrailing()
		o.byte('\n')
		return end
	}
	if atLineEnd {
		o.trimTrailing()
		return restOfLine
	}
	if c := o.lastByte(); c != ' ' && c != '\t' && c != '\n' {
		o.byte(' ')
	}
	return restOfLine
}

var regexKeywords = map[string]bool{
	"return": true, "typeof": true, "case": true, "do": true, "else": true, "in": true, "of": true,
	"new": true, "delete": true, "void": true, "throw": true, "instanceof": true, "yield": true, "await": true,
}

func StripJS(src string) string {
	o := &out{}
	lexJS(src, 0, o, false)
	return strings.TrimLeft(o.String(), "\n")
}

func lexJS(src string, i int, o *out, inBraces bool) int {
	depth := 0
	last := ""
	for i < len(src) {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				end = len(src)
			} else {
				end += i
			}
			i = skipComment(src, i, end, o)
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				end = len(src)
			} else {
				end += i + 4
			}
			i = skipComment(src, i, end, o)
		case c == '\'' || c == '"':
			j := scanQuoted(src, i)
			o.str(src[i:j])
			i, last = j, "str"
		case c == '`':
			i = lexTemplate(src, i, o)
			last = "str"
		case c == '/':
			if regexAllowed(last) {
				j := scanRegex(src, i)
				o.str(src[i:j])
				i, last = j, "regex"
			} else {
				o.byte(c)
				i, last = i+1, "/"
			}
		case isIdentStart(c):
			j := i + 1
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			word := src[i:j]
			o.str(word)
			i, last = j, word
			if !regexKeywords[word] {
				last = "id"
			}
		case c >= '0' && c <= '9':
			j := i + 1
			for j < len(src) && (isIdentPart(src[j]) || src[j] == '.') {
				j++
			}
			o.str(src[i:j])
			i, last = j, "num"
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			o.byte(c)
			i++
		default:
			if inBraces {
				if c == '{' {
					depth++
				} else if c == '}' {
					if depth == 0 {
						return i
					}
					depth--
				}
			}
			o.byte(c)
			i, last = i+1, string(c)
		}
	}
	return i
}

func regexAllowed(last string) bool {
	switch last {
	case "", "(", ",", "=", ":", "[", "!", "&", "|", "?", "{", "}", ";", "+", "-", "*", "%", "<", ">", "~", "^":
		return true
	case "id", "num", "str", "regex", ")", "]", "/":
		return false
	}
	return regexKeywords[last]
}

func scanQuoted(src string, i int) int {
	quote := src[i]
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case quote, '\n':
			return j + 1
		}
	}
	return len(src)
}

func lexTemplate(src string, i int, o *out) int {
	o.byte('`')
	i++
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\\' && i+1 < len(src):
			o.str(src[i : i+2])
			i += 2
		case c == '`':
			o.byte('`')
			return i + 1
		case c == '$' && i+1 < len(src) && src[i+1] == '{':
			o.str("${")
			i = lexJS(src, i+2, o, true)
			if i < len(src) {
				o.byte('}')
				i++
			}
		default:
			o.byte(c)
			i++
		}
	}
	return i
}

func scanRegex(src string, i int) int {
	inClass := false
	for j := i + 1; j < len(src); j++ {
		switch c := src[j]; {
		case c == '\\':
			j++
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		case c == '\n':
			return j
		case c == '/' && !inClass:
			j++
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			return j
		}
	}
	return len(src)
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }

func StripCSS(src string) string {
	o := &out{}
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				end = len(src)
			} else {
				end += i + 4
			}
			i = skipComment(src, i, end, o)
		case c == '"' || c == '\'':
			j := scanQuoted(src, i)
			o.str(src[i:j])
			i = j
		default:
			o.byte(c)
			i++
		}
	}
	return strings.TrimLeft(o.String(), "\n")
}

func StripMarkup(src string) string {
	o := &out{}
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				end = len(src)
			} else {
				end += i + 7
			}
			i = skipComment(src, i, end, o)
			continue
		}
		o.byte(src[i])
		i++
	}
	return o.String()
}

func StripHashLines(src string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(src, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if b.Len() == 0 && strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}
