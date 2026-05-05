package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{
			name:  "empty input",
			input: "",
			width: 80,
			want:  "",
		},
		{
			name:  "zero width is no-op",
			input: "long line that would otherwise wrap somewhere",
			width: 0,
			want:  "long line that would otherwise wrap somewhere",
		},
		{
			name:  "short line untouched",
			input: "hello world",
			width: 80,
			want:  "hello world",
		},
		{
			name:  "wraps on space at column boundary",
			input: "aaa bbb ccc",
			width: 7,
			// "aaa bbb" fits in 7; next word forces wrap.
			want: "aaa bbb\nccc",
		},
		{
			name:  "preserves existing newlines as paragraph breaks",
			input: "line one is short\n\nline two is also short",
			width: 80,
			want:  "line one is short\n\nline two is also short",
		},
		{
			name:  "wraps each line independently",
			input: "first first first first\nsecond second second second",
			width: 12,
			// "first first" = 11 chars fits in 12; two "second"s would be 13 chars so each gets its own line.
			want: "first first\nfirst first\nsecond\nsecond\nsecond\nsecond",
		},
		{
			name:  "single oversize token overflows rather than splitting",
			input: "https://example.com/a/very/long/path/that/exceeds/the/wrap/width tail",
			width: 20,
			// URL stays intact on its own line; "tail" wraps to next line.
			want: "https://example.com/a/very/long/path/that/exceeds/the/wrap/width\ntail",
		},
		{
			name:  "markdown link kept on one line",
			input: "see [the documentation here](https://example.com/docs) for details",
			width: 30,
			// The whole [label](url) is one token; it overflows its line, then "for details" wraps after.
			want: "see\n[the documentation here](https://example.com/docs)\nfor details",
		},
		{
			name:  "multi-space gaps still allow packing within width",
			input: "aa  bb  cc",
			width: 5,
			// "aa bb" packs to 5 chars (excess space collapsed at the wrap-decision step); "cc" wraps.
			want: "aa bb\ncc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.input, tt.width)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWrapText_NoLineExceedsWidthExceptOversizeTokens(t *testing.T) {
	// Property check: for normal prose every wrapped line is <= width, and any
	// line longer than width contains zero spaces (i.e. it's a single token).
	input := strings.Repeat("alpha beta gamma delta epsilon zeta eta theta ", 10)
	width := 40
	out := wrapText(input, width)
	for _, line := range strings.Split(out, "\n") {
		if len(line) > width {
			assert.NotContains(t, line, " ", "over-width line %q must be a single token", line)
		}
	}
}
