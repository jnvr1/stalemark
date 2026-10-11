package shellx

import (
	"slices"
	"testing"
)

func TestStripPrompts(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		console bool
		want    []string
	}{
		{"no prompt keeps everything", []string{"make build", "# c", "  out"}, false, []string{"make build", "# c", "  out"}},
		{"console without prompts is all output", []string{"build ok"}, true, []string{""}},
		{"dollar lines are commands, the rest output", []string{"$ go test", "ok", "$", "$ ls"}, false, []string{"go test", "", "", "ls"}},
		{"secondary prompt after backslash", []string{"$ a \\", ">   b \\", "> c", "out"}, false, []string{"a \\", "  b \\", "c", ""}},
		{"secondary prompt without backslash is output", []string{"$ npm test", "> demo test", "> jest"}, false, []string{"npm test", "", ""}},
		{"bare continuation after backslash", []string{"$ a \\", "  b", "out"}, false, []string{"a \\", "  b", ""}},
		{"heredoc body with secondary prompts", []string{"$ cat <<EOF", "> x", "> EOF", "x"}, false, []string{"cat <<EOF", "x", "EOF", ""}},
		{"heredoc body without prompts", []string{"$ cat <<-'EOF' > f", "\tbody", "\tEOF", "out"}, false, []string{"cat <<-'EOF' > f", "\tbody", "\tEOF", ""}},
		{"here-string is not a heredoc", []string{"$ cat <<<foo", "out"}, false, []string{"cat <<<foo", ""}},
		{"empty", nil, false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripPrompts(tt.in, tt.console)
			if len(got) != len(tt.in) {
				t.Fatalf("len = %d, want %d (line count must be preserved)", len(got), len(tt.in))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("stripPrompts(%q, %v) = %q, want %q", tt.in, tt.console, got, tt.want)
			}
		})
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"git clone <repo-url>", "git clone __________"},
		{"cd <project dir>", "cd _____________"},
		{"a<b>", "a___"},
		{"<a> <b>", "___ ___"},
		{"cat <<EOF > file", "cat <<EOF > file"},
		{"<<EOF", "<<EOF"},
		{"<1x>", "<1x>"},
		{"<>", "<>"},
		{"cmd 2>&1 </dev/null", "cmd 2>&1 </dev/null"},
		{"x", "x"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := mask(tt.in)
			if got != tt.want {
				t.Errorf("mask(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if len(got) != len(tt.in) {
				t.Errorf("mask(%q) changed length %d -> %d", tt.in, len(tt.in), len(got))
			}
		})
	}
}
