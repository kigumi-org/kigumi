package doc

import (
	"html"
	"sort"
	"strings"
)

// HTML renders the Markdown pages as a static site: one page per package
// plus index.html, styled inline so the output needs no other file. The
// Markdown is our own (headings, fenced code, list items, paragraphs), so
// a small converter covers it.
func HTML(pages map[string]string, title string) map[string]string {
	paths := make([]string, 0, len(pages))
	for p := range pages {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := map[string]string{}
	nav := navHTML(paths)
	for _, p := range paths {
		out[p+".html"] = page(p+" - "+title, nav, markdownToHTML(pages[p]))
	}
	var index strings.Builder
	index.WriteString("<h1>" + html.EscapeString(title) + "</h1>\n<ul>\n")
	for _, p := range paths {
		index.WriteString("<li><a href=\"" + html.EscapeString(p) + ".html\">" + html.EscapeString(p) + "</a></li>\n")
	}
	index.WriteString("</ul>\n")
	out["index.html"] = page(title, nav, index.String())
	return out
}

func navHTML(paths []string) string {
	var sb strings.Builder
	sb.WriteString("<nav><a href=\"index.html\">Packages</a>\n")
	for _, p := range paths {
		sb.WriteString("<a href=\"" + html.EscapeString(p) + ".html\">" + html.EscapeString(p) + "</a>\n")
	}
	sb.WriteString("</nav>\n")
	return sb.String()
}

const style = `body{margin:0;font:15px/1.5 system-ui,sans-serif;color:#1f2328;background:#fff}
main{max-width:56rem;margin:0 auto;padding:1.5rem 2rem 4rem}
nav{border-bottom:1px solid #d0d7de;padding:.5rem 2rem;display:flex;flex-wrap:wrap;gap:.25rem 1rem;font-size:14px}
a{color:#0969da;text-decoration:none}a:hover{text-decoration:underline}
h1{font-size:1.8rem}h2{border-bottom:1px solid #d0d7de;padding-bottom:.25rem;margin-top:2rem}
h3{margin-top:1.5rem}h4{margin:1rem 0 0 1rem}
pre{background:#f6f8fa;border-radius:6px;padding:.75rem 1rem;overflow-x:auto}
code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.92em}
p code{background:#f6f8fa;border-radius:4px;padding:0 .25em}
@media(prefers-color-scheme:dark){body{color:#e6edf3;background:#0d1117}nav{border-color:#30363d}h2{border-color:#30363d}pre,p code{background:#161b22}a{color:#58a6ff}}`

func page(title, nav, body string) string {
	return "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n<title>" +
		html.EscapeString(title) + "</title>\n<style>" + style + "</style>\n</head>\n<body>\n" + nav + "<main>\n" + body + "</main>\n</body>\n</html>\n"
}

// markdownToHTML converts the subset of Markdown the renderer emits.
func markdownToHTML(md string) string {
	var sb strings.Builder
	lines := strings.Split(md, "\n")
	inCode, inList, inPara := false, false, false
	closePara := func() {
		if inPara {
			sb.WriteString("</p>\n")
			inPara = false
		}
	}
	closeList := func() {
		if inList {
			sb.WriteString("</ul>\n")
			inList = false
		}
	}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "```"):
			closePara()
			closeList()
			if inCode {
				sb.WriteString("</code></pre>\n")
			} else {
				sb.WriteString("<pre><code>")
			}
			inCode = !inCode
		case inCode:
			sb.WriteString(html.EscapeString(line) + "\n")
		case strings.HasPrefix(line, "#"):
			closePara()
			closeList()
			level := len(line) - len(strings.TrimLeft(line, "#"))
			text := strings.TrimSpace(line[level:])
			sb.WriteString("<h" + string(rune('0'+level)) + " id=\"" + html.EscapeString(anchor(text)) + "\">" + inline(text) + "</h" + string(rune('0'+level)) + ">\n")
		case strings.HasPrefix(line, "- "):
			closePara()
			if !inList {
				sb.WriteString("<ul>\n")
				inList = true
			}
			sb.WriteString("<li>" + inline(strings.TrimPrefix(line, "- ")) + "</li>\n")
		case strings.TrimSpace(line) == "":
			closePara()
			closeList()
		default:
			closeList()
			if !inPara {
				sb.WriteString("<p>")
				inPara = true
			} else {
				sb.WriteString(" ")
			}
			sb.WriteString(inline(line))
		}
	}
	closePara()
	closeList()
	if inCode {
		sb.WriteString("</code></pre>\n")
	}
	return sb.String()
}

// inline escapes text and turns `code` spans and [text](href) links into
// their HTML, the two inline forms the doc comments and the index use.
func inline(text string) string {
	var sb strings.Builder
	for len(text) > 0 {
		switch {
		case text[0] == '`':
			end := strings.IndexByte(text[1:], '`')
			if end < 0 {
				sb.WriteString(html.EscapeString(text))
				return sb.String()
			}
			sb.WriteString("<code>" + html.EscapeString(text[1:1+end]) + "</code>")
			text = text[end+2:]
		case text[0] == '[':
			mid := strings.Index(text, "](")
			end := strings.IndexByte(text, ')')
			if mid < 0 || end < mid {
				sb.WriteString(html.EscapeString(text[:1]))
				text = text[1:]
				continue
			}
			href := strings.TrimSuffix(text[mid+2:end], ".md") + ".html"
			sb.WriteString("<a href=\"" + html.EscapeString(href) + "\">" + html.EscapeString(text[1:mid]) + "</a>")
			text = text[end+1:]
		default:
			i := strings.IndexAny(text, "`[")
			if i < 0 {
				i = len(text)
			}
			sb.WriteString(html.EscapeString(text[:i]))
			text = text[i:]
		}
	}
	return sb.String()
}

func anchor(text string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			sb.WriteRune(r)
		case r == ' ' || r == '-':
			sb.WriteByte('-')
		}
	}
	return sb.String()
}
