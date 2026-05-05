package main

import (
	"strings"
	"unicode/utf8"
)

// previewWrapWidth is the column at which preview body text is wrapped.
// Chosen to roughly match the visual width of an image preview rendered
// alongside the text in Mattermost's web client.
const previewWrapWidth = 80

// wrapText hard-wraps text to the given column width. Each existing line
// (split on \n) is wrapped independently, so explicit blank lines / single
// breaks from the source post are preserved.
//
// Behavior:
//   - Splits on whitespace; never splits a single token (long URLs overflow
//     their line rather than getting broken in the middle).
//   - Markdown links of the form [label](url) are kept on a single line — we
//     never break between the closing ] and opening (.
//   - CJK or other unspaced text falls through unchanged (one "token").
func wrapText(text string, width int) string {
	if width <= 0 || text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = wrapLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func wrapLine(line string, width int) string {
	if utf8.RuneCountInString(line) <= width {
		return line
	}

	tokens := tokenize(line)
	var out strings.Builder
	col := 0
	for i, tok := range tokens {
		tokLen := utf8.RuneCountInString(tok.text)
		if tok.isSpace {
			// Collapse a run of spaces into either a single space or a wrap.
			if col == 0 {
				// Leading whitespace on a fresh line — preserve it.
				out.WriteString(tok.text)
				col += tokLen
				continue
			}
			// Look ahead at the next non-space token to decide whether
			// the upcoming word still fits.
			next := nextNonSpace(tokens, i+1)
			if next == nil {
				// Trailing whitespace, drop it.
				continue
			}
			if col+1+utf8.RuneCountInString(next.text) > width {
				out.WriteByte('\n')
				col = 0
			} else {
				out.WriteByte(' ')
				col++
			}
			continue
		}
		// Non-space token.
		if col == 0 {
			out.WriteString(tok.text)
			col = tokLen
			continue
		}
		// At this point col > 0 means we already wrote at least one token
		// on this line and the previous space-token already decided whether
		// to wrap, so just append.
		out.WriteString(tok.text)
		col += tokLen
	}
	return out.String()
}

type wrapTok struct {
	text    string
	isSpace bool
}

// tokenize splits a line into alternating space and non-space runs, with
// markdown links [label](url) kept as a single non-space token so they never
// get broken across a wrap.
func tokenize(line string) []wrapTok {
	var toks []wrapTok
	i := 0
	for i < len(line) {
		r := line[i]
		switch {
		case r == ' ' || r == '\t':
			j := i
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			toks = append(toks, wrapTok{text: line[i:j], isSpace: true})
			i = j
		case r == '[':
			// Try to consume a full markdown link [label](url) as one token.
			if end := matchMarkdownLink(line, i); end > i {
				toks = append(toks, wrapTok{text: line[i:end]})
				i = end
				continue
			}
			fallthrough
		default:
			j := i
			for j < len(line) && line[j] != ' ' && line[j] != '\t' {
				if line[j] == '[' && j != i {
					break
				}
				j++
			}
			toks = append(toks, wrapTok{text: line[i:j]})
			i = j
		}
	}
	return toks
}

// matchMarkdownLink returns the index just past the closing ')' of a
// well-formed [label](url) starting at start, or start if the syntax doesn't
// match. Nested brackets in the label aren't supported (rare in post text).
func matchMarkdownLink(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return start
	}
	closeBracket := strings.IndexByte(s[start+1:], ']')
	if closeBracket < 0 {
		return start
	}
	closeBracket += start + 1
	if closeBracket+1 >= len(s) || s[closeBracket+1] != '(' {
		return start
	}
	closeParen := strings.IndexByte(s[closeBracket+2:], ')')
	if closeParen < 0 {
		return start
	}
	return closeBracket + 2 + closeParen + 1
}

func nextNonSpace(toks []wrapTok, from int) *wrapTok {
	for k := from; k < len(toks); k++ {
		if !toks[k].isSpace {
			return &toks[k]
		}
	}
	return nil
}
