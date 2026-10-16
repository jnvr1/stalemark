# 3. External tool orchestration for API docs checks

Date: 2026-10-15

## Status

Accepted

## Context

API docs drift (Pain B in `docs/research/02-feasibility-and-stack.md`) is
already covered by mature tools: oasdiff for OpenAPI spec vs spec, pydoclint
for Python docstrings vs signatures, eslint-plugin-jsdoc for JSDoc vs
JavaScript and TypeScript signatures. The plan is to wrap them, not rebuild
them, and to stay off archived projects (Optic, swagger-cli, darglint).

Facts checked on 2026-10-15:

- oasdiff v1.33.0, Apache-2.0, Go. Documented as embeddable: load both specs
  with the kin-openapi loader, then `diff.Get` and the `checker` package
  (`CheckBackwardCompatibility`, levels ERR, WARN, INFO). Its own releases are
  built with CGO_ENABLED=0, so it is pure Go.
- pydoclint 0.11.1, MIT, Python 3.10 or newer, installed with pip. It has no
  JSON output: native mode prints `file:line: DOCnnn: message` lines on stderr
  and exits 1 both on violations and on usage errors. Its baseline mode
  rewrites the baseline file by default.
- eslint-plugin-jsdoc v65.3.6. The GitHub API reports the license as
  NOASSERTION, but the LICENSE file and package.json say BSD-3-Clause. Peer
  dependency: eslint 7 to 10. ESLint's `json` formatter gives file, line,
  column and ruleId per message; exit 0 clean, 1 lint errors, 2 config or
  internal error.

Two of the three tools need a Python or Node runtime. ADR 0002 says stalemark
itself must not.

## Decision

### Discovery

- Every wrapper is opt-in under an `api` key of `.stalemark.yml`, for example
  `api: {oasdiff: {spec: ..., against: ...}, pydoclint: {...}, jsdoc: {...}}`.
  Without the key the wrapper does not run and costs nothing.
- External tools are looked up on PATH only, with Go's exec.LookPath (which
  honors PATHEXT on Windows and refuses current-directory hits). No
  auto-install, no download at check time, no node_modules or virtualenv
  probing. `npm exec` and an activated virtualenv already put the local tool on
  PATH, so that is the documented way to point stalemark at a project-local
  install.
- A missing tool is a tool error (exit 2) only when its wrapper is enabled. A
  tool that is installed but not enabled is never run.
- Rejected: bundling or downloading the tools (the pre-commit and MegaLinter
  model). It would make stalemark a package manager for Python and Node, break
  the single static binary of ADR 0002, and make results depend on whatever
  version was fetched that day, which is a precision and determinism problem.

### Pinning

- Each subprocess wrapper has a minimum version, defaulting to the version its
  parser fixtures were recorded with, overridable as a min-version key in the
  wrapper's config. stalemark runs the tool's version command first and exits 2
  with "pydoclint 0.9.0 is older than the required 0.11.1" style messages when
  it is older. No maximum: a newer tool is accepted and parser drift is caught
  by the fixtures below.
- Each parser targets one named output format: pydoclint native mode with
  filenames on every line, ESLint's built-in `json` formatter. stalemark always
  passes the format flags itself; the user's config cannot change them.
- Raw tool output is committed as golden fixtures under `testdata/<tool>/...`
  next to the inputs that produced it and the expected findings. Unit tests
  parse the raw output, so they run without the tool installed. A separate CI
  job installs the tools on ubuntu, regenerates the raw output and fails if it
  differs from the committed fixture: a format change shows up as a failing
  diff, not as silently missing findings.

### oasdiff: library, not subprocess

oasdiff is imported as a Go module. It is the only wrapper that stays inside
the single binary: no PATH lookup, no version check, the version is pinned by
`go.mod` and the output is Go structs, not text to parse. The cost is the
dependency tree (kin-openapi and its YAML and JSON helpers); its child issue
must measure the binary size and dependency delta before merging. Specs are
read from files only; URLs and git revisions are rejected so that a check never
touches the network.

### Output normalization

Each wrapper maps tool output to the existing Finding of
`internal/finding/finding.go`:

- Rule: a stalemark id in a new SM-B range (SM-B00x oasdiff, SM-B01x
  pydoclint, SM-B02x eslint-plugin-jsdoc), mapped from a fixed set of tool
  codes. Tool codes outside the set are dropped, not passed through.
- Path: the source or spec file, relative to the check root, forward slashes.
  Line and Col from the tool; Col 0 when the tool has none (pydoclint).
- Message: prefixed with the tool name and its own code, for example
  "pydoclint DOC101: ...". Subject: the symbol, route or parameter. Suggestion:
  only when the tool gives one.
- Fingerprints stay sha256(Rule, Path, Subject), so baselines work unchanged.
- SARIF: one run, stalemark as driver, rules in the driver's catalog in
  `internal/report/sarif.go`, plus one `tool.extensions` entry per wrapped tool
  that ran, with its name and version. SARIF 2.1.0 (section 3.18.3) defines
  extensions as the components a tool used during the run. Separate runs are
  rejected: GitHub code scanning stopped accepting files with several runs of
  the same tool and category in July 2025, and the rules are stalemark's anyway.
- Wrappers run once per check, in the repo-level pass next to the env code
  scan, not once per document.

### Severity

Wrapped findings default to warning: the tool's precision is not ours, and a
docstring mismatch is a weaker claim than a missing file. Each wrapper takes a
severity key (note, warning or error). Note keeps the findings without failing
the run.

## Consequences

- stalemark has runtime dependencies for the first time, but only for users who
  enable a subprocess wrapper. The README lists, per wrapper, the tool, the
  minimum version and how to install it.
- The end-to-end fixtures and the default CI jobs stay tool-free. Only the new
  tools job installs oasdiff, pydoclint, eslint and eslint-plugin-jsdoc.
- Wrapper output is only as good as the tool: false positives from a tool are
  handled by its own ignore mechanism (noqa, eslint-disable) or by the stalemark
  baseline, never by heuristics layered on top.
- Wrappers never write into the repository. pydoclint is run with automatic
  baseline regeneration turned off for that reason.
- Adding a wrapper means a config key, a min-version, a code-to-rule map, raw
  output fixtures and SARIF catalog entries. Anything that cannot be mapped
  deterministically to a file and line is out of scope.
