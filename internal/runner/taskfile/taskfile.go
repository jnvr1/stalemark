// Package taskfile checks task invocations against a statically parsed
// Taskfile.yml (Task v3). task is never executed.
//
// ponytail: conservative mode. Task names built from templates, remote or
// missing includes and non-v3 files define tasks we cannot see, so those
// namespaces (or the whole file) report nothing; upgrade path is an opt-in
// "task --list-all --json" listing.
package taskfile

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// Rules reported by this resolver.
const (
	RuleTaskNotFound = "SM-A003" // the task is not defined in the Taskfile
	RuleTaskInternal = "SM-A004" // the task exists but is internal: true
)

// Resolver implements runner.Resolver for task.
type Resolver struct{}

// New returns the task resolver.
func New() Resolver { return Resolver{} }

// Names lists the tools this resolver handles.
func (Resolver) Names() []string { return []string{"task"} }

// Resolve reports one finding per task of cmd that its Taskfile lacks or
// marks internal.
func (Resolver) Resolve(ctx runner.Context, doc string, cmd shellx.Command) []finding.Finding {
	inv := parseArgs(cmd.Args)
	if inv.silent || len(inv.names) == 0 {
		return nil
	}
	dir := filepath.Join(runner.WorkDir(ctx, cmd), filepath.FromSlash(inv.dir))
	path := discover(dir, inv.file)
	if path == "" {
		return nil
	}
	tf := load(path, 0, map[string]bool{})
	if tf == nil || tf.opaque {
		return nil
	}
	rel := runner.Rel(ctx, path)
	var out []finding.Finding
	for _, name := range inv.names {
		f := finding.Finding{Severity: finding.Error, Path: doc, Line: cmd.FileLine, Subject: name}
		switch internal, ok := tf.tasks[name]; {
		case ok && !internal, tf.opaqueNS(name):
			continue
		case ok:
			f.Rule = RuleTaskInternal
			f.Message = fmt.Sprintf("task %q is internal and cannot be run from the command line", name)
		default:
			f.Rule = RuleTaskNotFound
			f.Suggestion = runner.Closest(name, tf.callable())
			f.Message = fmt.Sprintf("task %q not found in %s", name, rel)
			if f.Suggestion != "" {
				f.Message += fmt.Sprintf(" (did you mean %q?)", f.Suggestion)
			}
		}
		out = append(out, f)
	}
	return out
}

// invocation is what matters of a task call: the -d directory, the -t file
// and the task names. silent is set by flags that never run a project task
// (--list, --init, -g...).
type invocation struct {
	dir, file string
	names     []string
	silent    bool
}

var (
	// options whose value is the next argument (or attached with =).
	valued = map[string]bool{
		"-t": true, "--taskfile": true, "-d": true, "--dir": true, "-o": true, "--output": true,
		"-C": true, "--concurrency": true, "-I": true, "--interval": true, "--sort": true,
		"--output-group-begin": true, "--output-group-end": true,
	}
	// flags after which no task of the project Taskfile is run.
	silentFlags = map[string]bool{
		"-g": true, "--global": true, "-l": true, "--list": true, "-a": true, "--list-all": true,
		"--init": true, "--version": true, "-h": true, "--help": true, "--summary": true, "--status": true,
	}
)

// parseArgs walks task's arguments: "--" ends the task names, NAME=VALUE
// tokens are variables and other dashed tokens are flags.
func parseArgs(args []string) invocation {
	var inv invocation
	for i := 0; i < len(args); i++ {
		a := args[i]
		opt, val, hasEq := strings.Cut(a, "=")
		switch {
		case a == "--":
			return inv
		case valued[opt]:
			if !hasEq && i+1 < len(args) {
				i++
				val = args[i]
			}
			switch opt {
			case "-t", "--taskfile":
				inv.file = val
			case "-d", "--dir":
				inv.dir = val
			}
		case silentFlags[a]:
			inv.silent = true
		case strings.HasPrefix(a, "-"), hasEq:
		default:
			inv.names = append(inv.names, a)
		}
	}
	return inv
}

// fileNames is Task's search order within a directory.
var fileNames = []string{
	"Taskfile.yml", "taskfile.yml", "Taskfile.yaml", "taskfile.yaml",
	"Taskfile.dist.yml", "taskfile.dist.yml", "Taskfile.dist.yaml", "taskfile.dist.yaml",
}

// discover returns the Taskfile task would read in dir: file (a file, or a
// directory holding one) when given, otherwise the first of fileNames.
// "" when none exists.
//
// ponytail: task also walks up parent directories; we stop at dir.
func discover(dir, file string) string {
	if file == "" {
		return runner.Discover(dir, fileNames...)
	}
	return locate(filepath.Join(dir, filepath.FromSlash(file)))
}

// locate resolves a Taskfile reference: the file itself, or the Taskfile
// inside the directory. "" when missing.
func locate(p string) string {
	st, err := os.Stat(p)
	switch {
	case err != nil:
		return ""
	case st.IsDir():
		return runner.Discover(p, fileNames...)
	}
	return p
}

// parsed is the static view of a Taskfile: every callable name (root tasks,
// aliases, ns:task, ns:alias and bare "ns:") mapped to whether it is
// internal. opaque means a root task name is templated: anything could
// match. opaques lists namespace prefixes ("ns:") whose tasks are unknown:
// remote or missing includes.
type parsed struct {
	tasks   map[string]bool
	opaque  bool
	opaques []string
}

// opaqueNS reports whether name belongs to a namespace we cannot inspect.
func (p *parsed) opaqueNS(name string) bool {
	return slices.ContainsFunc(p.opaques, func(ns string) bool { return strings.HasPrefix(name, ns) })
}

// callable is the sorted list of names a user may type, for suggestions.
// Bare "ns:" entries are left out: they are a prefix of every ns task.
func (p *parsed) callable() []string {
	var out []string
	for n, internal := range p.tasks {
		if !internal && !strings.HasSuffix(n, ":") {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// load parses path and its local includes. nil when the file is unreadable,
// not YAML, not version 3, part of a cycle or nested too deep.
func load(path string, depth int, seen map[string]bool) *parsed {
	abs, err := filepath.Abs(path)
	if err != nil || seen[abs] || depth > 8 {
		return nil
	}
	seen[abs] = true
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	p, includes := parseSource(src)
	if p == nil {
		return nil
	}
	for ns, v := range includes {
		inc := parseInclude(v)
		prefixes := append([]string{ns + ":"}, suffixed(inc.aliases, ":")...)
		var sub *parsed
		if !inc.remote() {
			target := filepath.Join(filepath.Dir(path), filepath.FromSlash(inc.path))
			if inc.optional && !exists(target) {
				continue
			}
			sub = load(locate(target), depth+1, seen)
		}
		if sub == nil || sub.opaque {
			p.opaques = append(p.opaques, prefixes...)
			continue
		}
		for _, pre := range prefixes {
			p.tasks[pre] = false // "ns:" runs the include's default task
			for n, internal := range sub.tasks {
				p.tasks[pre+n] = internal || inc.internal
			}
			for _, o := range sub.opaques {
				p.opaques = append(p.opaques, pre+o)
			}
		}
	}
	return p
}

// parseSource reads the tasks and raw includes of one Taskfile. nil unless
// it is a v3 file.
//
// ponytail: loose map[string]any decoding and type switches instead of the
// full Taskfile schema; add fields here as checks need them.
func parseSource(src []byte) (*parsed, map[string]any) {
	var doc struct {
		Version  any            `yaml:"version"`
		Includes map[string]any `yaml:"includes"`
		Tasks    map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil || !isV3(doc.Version) {
		return nil, nil
	}
	p := &parsed{tasks: map[string]bool{}}
	for name, v := range doc.Tasks {
		if strings.Contains(name, "{{") {
			p.opaque = true
			continue
		}
		internal, _ := field(v, "internal").(bool)
		p.tasks[name] = internal
		for _, a := range strs(field(v, "aliases")) {
			p.tasks[a] = internal
		}
	}
	return p, doc.Includes
}

// isV3 accepts "3", 3, 3.0 and "3.x" versions.
func isV3(v any) bool {
	s := fmt.Sprint(v)
	return s == "3" || strings.HasPrefix(s, "3.")
}

// include is the subset of an includes: entry that affects lookups.
type include struct {
	path               string
	optional, internal bool
	aliases            []string
}

func (inc include) remote() bool {
	return strings.Contains(inc.path, "://") || strings.HasPrefix(inc.path, "git@") ||
		strings.HasPrefix(inc.path, "git+") || strings.Contains(inc.path, "{{")
}

// parseInclude reads the string or map form of an include.
func parseInclude(v any) include {
	if s, ok := v.(string); ok {
		return include{path: s}
	}
	inc := include{aliases: strs(field(v, "aliases"))}
	inc.path, _ = field(v, "taskfile").(string)
	inc.optional, _ = field(v, "optional").(bool)
	inc.internal, _ = field(v, "internal").(bool)
	return inc
}

// field is v[key] when v is a map, nil otherwise.
func field(v any, key string) any {
	m, _ := v.(map[string]any)
	return m[key]
}

// strs is the string items of a YAML sequence.
func strs(v any) []string {
	var out []string
	items, _ := v.([]any)
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func suffixed(ss []string, suffix string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s + suffix
	}
	return out
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
