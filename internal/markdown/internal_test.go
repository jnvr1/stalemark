package markdown

import "testing"

func TestSplitInfo(t *testing.T) {
	tests := []struct {
		info, wantLang, wantMeta string
	}{
		{"bash", "bash", ""},
		{`bash title="x"`, "bash", `title="x"`},
		{"Go", "go", ""},
		{"  sh  ", "sh", ""},
		{"sh\tfoo  bar", "sh", "foo  bar"},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.info, func(t *testing.T) {
			lang, meta := splitInfo(tt.info)
			if lang != tt.wantLang || meta != tt.wantMeta {
				t.Errorf("splitInfo(%q) = (%q, %q), want (%q, %q)", tt.info, lang, meta, tt.wantLang, tt.wantMeta)
			}
		})
	}
}

func TestStripFrontMatter(t *testing.T) {
	tests := []struct {
		name, src, wantBody string
		wantSkipped         int
	}{
		{"lf", "---\na: 1\n---\nbody", "body", 3},
		{"crlf", "---\r\na: 1\r\n---\r\nbody", "body", 3},
		{"empty block", "---\n---\n", "", 2},
		{"unclosed", "---\na: 1\n", "---\na: 1\n", 0},
		{"not at byte 0", "\n---\n---\n", "\n---\n---\n", 0},
		{"four dashes", "----\n---\n", "----\n---\n", 0},
		{"no front matter", "x", "x", 0},
		{"empty", "", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, skipped := stripFrontMatter([]byte(tt.src))
			if string(body) != tt.wantBody || skipped != tt.wantSkipped {
				t.Errorf("stripFrontMatter(%q) = (%q, %d), want (%q, %d)", tt.src, body, skipped, tt.wantBody, tt.wantSkipped)
			}
		})
	}
}

func TestLineIndex(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		skipped int
		off     int
		wantLn  int
		wantCol int
	}{
		{"first byte", "ab\ncd\n", 0, 0, 1, 1},
		{"newline belongs to its line", "ab\ncd\n", 0, 2, 1, 3},
		{"first byte of second line", "ab\ncd\n", 0, 3, 2, 1},
		{"second byte of second line", "ab\ncd\n", 0, 4, 2, 2},
		{"crlf: cr belongs to its line", "ab\r\ncd", 0, 2, 1, 3},
		{"crlf: second line", "ab\r\ncd", 0, 4, 2, 1},
		{"no trailing newline: last byte", "ab\ncd", 0, 4, 2, 2},
		{"no trailing newline: end of body", "ab\ncd", 0, 5, 2, 3},
		{"front matter offset", "x", 3, 0, 4, 1},
		{"empty body", "", 0, 0, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := newLineIndex([]byte(tt.body), tt.skipped)
			line, col := ix.lineCol(tt.off)
			if line != tt.wantLn || col != tt.wantCol {
				t.Errorf("lineCol(%d) = (%d, %d), want (%d, %d)", tt.off, line, col, tt.wantLn, tt.wantCol)
			}
			if got := ix.line(tt.off); got != tt.wantLn {
				t.Errorf("line(%d) = %d, want %d", tt.off, got, tt.wantLn)
			}
		})
	}
}
