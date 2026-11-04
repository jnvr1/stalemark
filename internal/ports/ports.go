// Package ports checks that the localhost ports a Markdown document names
// (localhost:3000, http://127.0.0.1:8080) are published by the repository's
// Dockerfiles (EXPOSE) or compose files (ports:). Precision first: no source,
// no published port or an interpolated port of unknown value means silence.
package ports

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jnvr1/stalemark/internal/finding"
	"github.com/jnvr1/stalemark/internal/markdown"
	"github.com/jnvr1/stalemark/internal/runner"
	"github.com/jnvr1/stalemark/internal/shellx"
)

// RulePortNotPublished is the rule ID of a documented localhost port that no
// Dockerfile or compose file publishes.
const RulePortNotPublished = "SM-N001"

var (
	composeNames = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	hostPortRE   = regexp.MustCompile(`(?i)\b(?:https?://)?(?:localhost|127\.0\.0\.1|0\.0\.0\.0):(\d{2,5})\b`)
	interpRE     = regexp.MustCompile(`\$\{[^}]*\}|\$[A-Za-z_]\w*`)
	defaultRE    = regexp.MustCompile(`^[A-Za-z_]\w*:?-(.*)$`)
)

// maxRange caps how many host ports one range entry expands to.
const maxRange = 1000

type mention struct {
	port      int
	line, col int
}

// published is the union of ports the repo's Docker files publish.
type published struct {
	ports    map[int]bool
	files    map[string]bool // root-relative paths of the files that contributed a port
	wildcard bool            // a port depends on an unset variable
}

// Check returns a SM-N001 finding for every documented localhost port of d
// that no Docker source publishes, once per port, in document order.
//
// ponytail: sources are read from the whole repo (no per-service lookup next to
// the doc) and prose mentions (paragraph text) are not scanned, only code
// spans, link targets and fenced blocks. Upgrade = nearest-Dockerfile lookup
// and a text scan of paragraphs.
func Check(ctx runner.Context, d *markdown.Doc) []finding.Finding {
	pub := readSources(ctx.Root)
	if pub.wildcard || len(pub.ports) == 0 {
		return nil
	}

	var ms []mention
	add := func(text string, line, col int) {
		for _, m := range hostPortRE.FindAllStringSubmatch(text, -1) {
			if p, _ := strconv.Atoi(m[1]); p >= 1 && p <= 65535 {
				ms = append(ms, mention{p, line, col})
			}
		}
	}
	for _, s := range d.Spans {
		add(s.Text, s.Line, s.Col)
	}
	for _, l := range d.Links {
		add(l.Target, l.Line, l.Col)
	}
	for _, b := range d.Blocks {
		if b.Ignored {
			continue
		}
		cl := containerLines(b)
		for _, l := range b.Lines {
			if !cl[l.FileLine] {
				add(l.Text, l.FileLine, 0)
			}
		}
	}
	sort.SliceStable(ms, func(i, j int) bool {
		return ms[i].line < ms[j].line || ms[i].line == ms[j].line && ms[i].col < ms[j].col
	})

	list := publishedList(pub)
	docPath := runner.Rel(ctx, d.Path)
	var out []finding.Finding
	seen := map[int]bool{}
	for _, m := range ms {
		if seen[m.port] || pub.ports[m.port] {
			continue
		}
		seen[m.port] = true
		f := finding.Finding{
			Rule: RulePortNotPublished, Severity: finding.Error, Path: docPath,
			Line: m.line, Col: m.col, Subject: strconv.Itoa(m.port),
			Message: fmt.Sprintf("port %d is documented but not published (%s)", m.port, list),
		}
		if len(pub.ports) == 1 {
			f.Suggestion = strconv.Itoa(slices.Sorted(maps.Keys(pub.ports))[0])
		}
		out = append(out, f)
	}
	return out
}

// containerLines returns the file lines of b taken by a docker, docker-compose,
// podman or kubectl command: localhost there is the container's loopback. A
// command spans from its first line up to the next command's line.
//
// ponytail: a `docker run ... && curl localhost:80` on one line is skipped too;
// upgrade = per-word positions from shellx.
func containerLines(b markdown.CodeBlock) map[int]bool {
	cmds := shellx.Extract(b)
	out := map[int]bool{}
	for i, c := range cmds {
		if !slices.Contains([]string{"docker", "docker-compose", "podman", "kubectl"}, c.Name) {
			continue
		}
		end := b.Lines[len(b.Lines)-1].FileLine + 1
		for _, n := range cmds[i+1:] {
			if n.FileLine > c.FileLine {
				end = n.FileLine
				break
			}
		}
		for l := c.FileLine; l < end; l++ {
			out[l] = true
		}
	}
	return out
}

// publishedList renders "compose.yml publishes 8080, 5432" with up to 5 ports.
func publishedList(p published) string {
	ports := slices.Sorted(maps.Keys(p.ports))
	var strs []string
	for _, n := range ports[:min(len(ports), 5)] {
		strs = append(strs, strconv.Itoa(n))
	}
	list := strings.Join(strs, ", ")
	if len(ports) > 5 {
		list += ", ..."
	}
	files := slices.Sorted(maps.Keys(p.files))
	if len(files) == 1 && strings.Contains(files[0], "Dockerfile") {
		return files[0] + " exposes " + list
	}
	return strings.Join(files, ", ") + " publishes " + list
}

// skipDirs are never walked for sources: pathref's generated directories plus
// testdata, so fixture repos do not leak into the repo being checked.
// ponytail: names copied from pathref (unexported); upgrade = share one list.
var skipDirs = []string{"node_modules", "dist", "build", "target", "out", "coverage", "vendor", ".git", "bin", "venv", ".venv", "env", "testdata"}

func isCompose(name string) bool {
	return slices.Contains(composeNames, name) || strings.HasSuffix(name, ".compose.yml") || strings.HasSuffix(name, ".compose.yaml")
}

// readSources collects the published ports of every Dockerfile and compose
// file under root, skipping skipDirs. files holds root-relative slash paths.
func readSources(root string) published {
	p := published{ports: map[int]bool{}, files: map[string]bool{}}
	_ = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := e.Name()
		if e.IsDir() {
			if path != root && slices.Contains(skipDirs, name) {
				return filepath.SkipDir
			}
			return nil
		}
		compose := isCompose(name)
		if !compose && name != "Dockerfile" && !strings.HasPrefix(name, "Dockerfile.") && !strings.HasSuffix(name, ".Dockerfile") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var got []int
		if compose {
			var wild bool
			got, wild = composePorts(src)
			p.wildcard = p.wildcard || wild
		} else {
			got = exposePorts(string(src))
		}
		for _, n := range got {
			p.ports[n] = true
		}
		if len(got) > 0 {
			rel, _ := filepath.Rel(root, path)
			p.files[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	return p
}

// exposePorts returns the ports of every EXPOSE line: "8080 9090/udp".
func exposePorts(src string) []int {
	var out []int
	for _, l := range strings.Split(src, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 || !strings.EqualFold(f[0], "EXPOSE") {
			continue
		}
		for _, tok := range f[1:] {
			if !strings.Contains(tok, "$") {
				out = append(out, portRange(strings.SplitN(tok, "/", 2)[0])...)
			}
		}
	}
	return out
}

// composePorts returns the host ports of every services.<name>.ports entry.
// wildcard reports an entry whose host port is an unset variable.
//
// ponytail: only the short and long ports syntax of the file itself; no
// extends, include, profiles or .env lookups. Upgrade = resolve ${VAR} from
// the .env file next to the compose file.
func composePorts(src []byte) (out []int, wildcard bool) {
	var doc struct {
		Services map[string]struct {
			Ports []any `yaml:"ports"`
		} `yaml:"services"`
	}
	if yaml.Unmarshal(src, &doc) != nil {
		return nil, false
	}
	for _, svc := range doc.Services {
		for _, item := range svc.Ports {
			var raw string
			switch v := item.(type) {
			case string:
				raw = v
			case map[string]any:
				if pub, ok := v["published"]; ok {
					raw = fmt.Sprint(pub)
				}
			}
			// Expand first: a default such as ${PORT:-3000} contains a colon.
			raw, ok := expand(raw)
			if !ok {
				wildcard = true
				continue
			}
			if _, isMap := item.(map[string]any); !isMap {
				raw = shortHost(raw)
			}
			out = append(out, portRange(raw)...)
		}
	}
	return out, wildcard
}

// shortHost returns the host part of "[[ip:]host:]container[/proto]", "" for a
// bare container port (random host port). Interpolations stay unexpanded.
func shortHost(s string) string {
	s = strings.SplitN(s, "/", 2)[0]
	if strings.HasPrefix(s, "[") { // [::1]:8080:80
		if i := strings.Index(s, "]:"); i >= 0 {
			s = s[i+2:]
		}
	}
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		return parts[0]
	case 3:
		return parts[1]
	}
	return ""
}

// expand resolves ${VAR:-default} and ${VAR-default} to the default; any other
// interpolation has no known value and reports ok=false.
func expand(s string) (string, bool) {
	ok := true
	s = interpRE.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "${") {
			if d := defaultRE.FindStringSubmatch(m[2 : len(m)-1]); d != nil {
				return d[1]
			}
		}
		ok = false
		return ""
	})
	return s, ok
}

// portRange parses "8080" or "8000-8010" (capped at maxRange entries);
// anything else yields nothing.
func portRange(s string) []int {
	lo, hi, found := strings.Cut(strings.TrimSpace(s), "-")
	a, err := strconv.Atoi(lo)
	if err != nil {
		return nil
	}
	b := a
	if found {
		if b, err = strconv.Atoi(hi); err != nil || b < a {
			return nil
		}
	}
	var out []int
	for n := a; n <= b && len(out) < maxRange; n++ {
		out = append(out, n)
	}
	return out
}
