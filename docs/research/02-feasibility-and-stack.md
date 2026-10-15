# Documentation-drift CLI: feasibility and stack research

Research date: 2026-10-10. All repo stats come from the GitHub REST API on that date (e.g. `https://api.github.com/repos/<owner>/<repo>`). Effort sizes (S/M/L/XL) are my own estimates for a solo maintainer building an MVP; they are judgement, not sourced facts. Claims I could not confirm are marked **[unverified]**.

---

## 0. Read this first: the space is already being squatted

Two very young, deterministic projects already attack exactly this problem. Neither has traction, but they define the minimum bar and the naming collisions.

| Project | What it does | Maturity | Source |
|---|---|---|---|
| `Allan-Nava/stalecheck` | Deterministic ("no model, no network"). Checks: dead file links, `path:line` past EOF, bad `#anchors`, `npm run X` with no such script, stale own-version, dated "verified" facts, unparsable `json` fences; optional: backticked paths, programs not on PATH. Outputs text, `--json`, `--sarif`, `--baseline`, `--fix`. Exit 1 on findings. | Created 2026-09-28, 0 stars, JavaScript, MIT, npm only | [README](https://github.com/Allan-Nava/stalecheck), [API](https://api.github.com/repos/Allan-Nava/stalecheck) |
| `thesatellite-ai/driftdoc` | Go CLI; sidecar `drift.yaml` of grep-style claims (contains/regex/absent/count). Exit 0/1/2, `--json`. AST, HTTP and CLI matchers are "planned". | Alpha, 1 commit, 0 stars, no license stated | [README](https://github.com/thesatellite-ai/driftdoc), [API](https://api.github.com/repos/thesatellite-ai/driftdoc) |
| `NicoSchwandner/docdrift` (npm `docdrift` 0.0.3) | "Link business decisions to source code. Detect when code drifts from documentation." | Tiny | [npm](https://registry.npmjs.org/docdrift) |
| `tomdevtech/docdrift` | Hybrid: git-diff detection + local LLM (Ollama) generates docs | "Under development", MIT | [README](https://github.com/tomdevtech/docdrift) |
| Swimm (commercial) | Code-coupled docs; CI check auto-syncs minor drift, fails build when a human is needed | Commercial SaaS | [Swimm docs](https://docs.swimm.io/features/keep-docs-updated-with-auto-sync/), [CI article](https://swimm.io/blog/continuous-documentation-through-continuous-integration-with-swimm) |

Implication: `stalecheck` already covers the "npm run X" slice of pain A in JS only. The open niche is a **language-agnostic** command/path/env/version validator (make, just, task, pyproject, cargo, go) with SARIF, baselines and stable exit codes. That is a differentiator, not a blank field.

---

## 1. Pain A: README commands that no longer run

### Deterministic check design
1. Parse each README/docs markdown file to an AST and collect fenced code blocks whose info string is `bash|sh|shell|console|zsh|powershell|pwsh|cmd` (plus unlabeled blocks when the first line starts with `$`). Keep the source position of each block so findings point at exact lines.
2. Strip prompts (`$ `, `> `), join `\` continuations, drop output lines in `console` blocks, split on `&&`, `;`, `|`.
3. Tokenize each command with a real shell parser (not regex) to get `argv`, env assignments, redirections and file arguments.
4. Resolve each command against a **registry of "runners"**, each a small deterministic resolver:
   - `npm|pnpm|yarn|bun run X` / `npm test|start` -> `package.json` `scripts` (JSON; trivial). `npx pkg` -> only flag if pkg is a local bin or listed dependency.
   - `make X` -> parse targets via `make -qp` (prints rule database, with file and line info, without running recipes) ([GNU make options](https://www.gnu.org/software/make/manual/html_node/Options-Summary.html)). Fallback: regex over `^target:`; `.PHONY`, includes and pattern rules are the hard part.
   - `just X` -> `just --summary` lists recipes ([just manual](https://raw.githubusercontent.com/casey/just/master/README.md)); `just --dump --dump-format json` gives a machine-readable form, but older notes say JSON needed `--unstable` **[verify on target version]** ([search result](https://just.systems/man/en/formatting-and-dumping-justfiles.html)).
   - `task X` -> parse `Taskfile.yml` (`tasks:` map; official [JSON schema](https://taskfile.dev/docs/reference/schema)) or `task --list --json` (documented with `--list`/`--list-all`) ([CLI ref](https://taskfile.dev/docs/reference/cli)). Includes and `--list` hiding tasks without `desc` are traps.
   - `poetry run X`, `uv run X`, `pdm run X`, `hatch run X` -> `pyproject.toml` `[project.scripts]`, `[tool.poetry.scripts]`, `[tool.pdm.scripts]`, `[tool.hatch.envs.*.scripts]` (TOML).
   - `cargo run --bin X|--example X|-p X`, `cargo xtask` -> `Cargo.toml` `[[bin]]`, `src/bin/*`, `examples/`, workspace members, `config.toml` aliases under `.cargo/`.
   - `go run ./cmd/x` / `go build ./...` / `go test ./pkg` -> check the directory exists and contains `package main` (for `run`).
   - `./script.sh`, `python path.py`, `node path.js`, `docker compose -f file` -> file-existence checks (and exec bit, optionally).
5. **CLI flag validation** (the hardest slice): for the project's own CLI, verify `--flag` names used in docs exist. Three ways, in order of determinism and cost:
   - (a) Static extraction by tree-sitter queries for `argparse.add_argument`, `commander .option()`, cobra `Flags().String(...)`, clap `#[arg(long)]`. Per-framework, brittle, many dynamic cases.
   - (b) Run the built binary's `--help` in CI and compare. Deterministic output, but requires a build and executes code.
   - (c) Flip the problem: generate the reference docs from the definitions and diff them, as cobra's `doc` package does with `GenMarkdownTree` ([pkg.go.dev](https://pkg.go.dev/github.com/spf13/cobra/doc)) or `clap-markdown` ([repo](https://github.com/ConnorGray/clap-markdown)). That is a **generated-docs freshness check**, not a README parser; mdsh's `--frozen` flag does the same for embedded command output ([mdsh](https://github.com/zimbatm/mdsh)).
6. Path references: backticked tokens that look like paths (`path/to/bar.ts`, `path/to/x.sh`) and markdown link targets -> `stat()`. Stalecheck gates backticked-path checks "off by default" (a signal that precision is a problem) ([README](https://github.com/Allan-Nava/stalecheck)).

### What the existing tools do (and why none fits)
- **Doc Detective**: executes tests declared in or inferred from docs, runs actions in a real browser; AGPL-3.0, Node 20/22, 136 stars, v4.38.1 (2026-08-13) ([repo](https://github.com/doc-detective/doc-detective), [releases](https://github.com/doc-detective/doc-detective/releases), [docs](https://docs.doc-detective.com/)). Execution-based, heavy, and AGPL is a poor dependency for embedding.
- **byexample**: runs examples embedded in Markdown and other text (`byexample -l python,ruby,shell README.md`); GPL-3.0; Windows only via WSL ([repo](https://github.com/byexamples/byexample)). Execution-based doctest style.
- **mdsh** (zimbatm): runs commands and splices output; `--frozen` fails if the output differs; MIT, Rust. A third-party crate page notes it "doesn't parse the Markdown file" and so can misinterpret commands ([repo](https://github.com/zimbatm/mdsh), [docs.rs](https://docs.rs/crate/mdsh/latest)).
- **Takeaway**: they all **execute** commands. A pure static check (no execution) is safer in CI and is the actual gap. Execution can be an opt-in `--exec` mode later.

### Libraries
| Need | Library | Notes / source |
|---|---|---|
| Markdown AST with positions | goldmark (Go), remark/mdast (TS), markdown-rs (Rust), pulldown-cmark (Rust) | See section 6 |
| Shell parsing | `mvdan/sh` (Go; POSIX/Bash/mksh/Zsh parser, BSD-3, 9.1k stars, active 2026-09-29) ([repo](https://github.com/mvdan/sh), [API](https://api.github.com/repos/mvdan/sh)); `tree-sitter-bash` (MIT; Rust/Node/Python bindings) ([repo](https://github.com/tree-sitter/tree-sitter-bash)) | PowerShell has no equally good Go parser **[unverified]**; treat PowerShell/cmd with simple tokenization and a smaller rule set |
| YAML/TOML/JSON manifests | stdlib/ecosystem parsers | Trivial |
| Makefile | `make -qp` output or tree-sitter-make **[unverified]** | `make -qp` includes file+line per recipe ([GNU make](https://www.gnu.org/software/make/manual/html_node/Options-Summary.html)) |
| Task / just | Parse YAML / call `just --summary` | See above |

### False-positive risks (this is where the project lives or dies)
- Illustrative or hypothetical commands (`npm run <your-script>`, `make deploy ENV=prod` in a "what you could do" section). Mitigation: skip placeholders (`<...>`, `$VAR`, `YOUR_`), support `<!-- driftcheck:ignore -->`, baseline file keyed on (check, doc, subject) like stalecheck's `--baseline` ([README](https://github.com/Allan-Nava/stalecheck)).
- Commands for **other** projects (install instructions for `brew`, `docker`, `kubectl`): only validate commands that resolve to a known local runner; unknown programs are not findings (stalecheck leaves "programs" off by default).
- Monorepos: `npm run build -w pkg`, `pnpm --filter`, nested Makefiles, `cd subdir && make x`. Needs working-directory tracking.
- Generated/dynamic targets: Makefile includes, pattern rules, Taskfile includes, `just` modules.
- Commands that create the file they reference later in the same block (`mkdir -p foo && cd foo`). Needs a tiny intra-block "files created" model.
- Windows/PowerShell blocks: syntax differs; limit checks to runner-name + script-name lookups.

### Effort
- MVP (markdown extraction, shell tokenization, runners: npm/make/just/task/pyproject/cargo/go, path existence, JSON+text+SARIF, baseline): **M** (about 3-5 weeks).
- Flag validation via tree-sitter per CLI framework: **L** each additional framework family; `--help` diffing mode: **S-M**.

---

## 2. Pain B: API docs that lie

### B1. OpenAPI vs implementation
Deterministic reality: OpenAPI-vs-code drift is **not** directly checkable statically across languages. Practical deterministic designs:
1. **Spec vs spec** (the reliable one): generate the spec from code in CI (FastAPI, NestJS, springdoc, tsoa, etc.), then diff against the committed spec. Tools:
   - **oasdiff**: Go, Apache-2.0, 1.4k stars, active (pushed 2026-10-09), compares two specs from files/URLs/git revisions, has a GitHub Action; does **not** compare against an implementation ([README](https://github.com/oasdiff/oasdiff), [API](https://api.github.com/repos/oasdiff/oasdiff), [action](https://api.github.com/repos/oasdiff/oasdiff-action)). Can be consumed as a Go library or a subprocess.
   - **openapi-diff** (OpenAPITools): Java, Apache-2.0, pushed 2026-09-04 ([API](https://api.github.com/repos/OpenAPITools/openapi-diff)). JVM dependency makes it a poor fit for a polyglot CLI.
   - **Optic**: MIT, **archived 2026-01-12** (read-only) ([repo](https://github.com/opticdev/optic), [API](https://api.github.com/repos/opticdev/optic)). Do not build on it. A third-party listing claims an Atlassian acquisition in 2024, which I could not verify ([listing](https://us.fitgap.com/products/009661/optic)).
2. **Spec vs live server**: Schemathesis (property-based tests from OpenAPI, CLI + pytest + GitHub Actions; MIT; very active, pushed 2026-10-10) ([docs](https://schemathesis.readthedocs.io/en/stable/), [API](https://api.github.com/repos/schemathesis/schemathesis)). Needs a running service: out of scope for a deterministic static CLI.
3. **Spec hygiene**: Spectral (Apache-2.0, active) and Redocly CLI (MIT, active) lint specs; `swagger-cli` is **archived** (2023) ([Spectral](https://api.github.com/repos/stoplightio/spectral), [Redocly](https://api.github.com/repos/Redocly/redocly-cli), [swagger-cli](https://api.github.com/repos/APIDevTools/swagger-cli)). These check spec quality, not truth vs code.
4. **Route-literal cross-check** (new, cheap, heuristic): extract route strings from code by framework patterns (Express `app.get('/x')`, FastAPI decorators, Gin, Spring annotations) with tree-sitter/ast-grep and compare with paths in the spec or markdown. Per-framework, partial coverage.

### B2. In-repo markdown API docs vs source signatures (tree-sitter)
Design: in markdown, find code spans/blocks like `foo(a, b)` or `GET /users/{id}`; extract identifiers; look up those symbols in a symbol index built with tree-sitter queries (function/method names, parameter lists, route decorators); report (i) symbol not found, (ii) arity/param-name mismatch, (iii) route not found.

Tree-sitter binding maturity (as of 2026-10-10):
| Runtime | Binding | Status | Source |
|---|---|---|---|
| Rust | `tree-sitter` crate 0.27.1, MIT, first-party | Best-supported, grammars as crates | [docs.rs](https://docs.rs/tree-sitter/latest/tree_sitter/), [repo API](https://api.github.com/repos/tree-sitter/tree-sitter) |
| Python | `py-tree-sitter` (MIT, pushed 2026-08-30) | Healthy | [API](https://api.github.com/repos/tree-sitter/py-tree-sitter) |
| Node | `node-tree-sitter` native (MIT, pushed 2026-07-29) and `web-tree-sitter` WASM; one `.wasm` per grammar, Node use is "considerably slower" than native | Healthy | [API](https://api.github.com/repos/tree-sitter/node-tree-sitter), [web-tree-sitter README](https://github.com/tree-sitter/tree-sitter/tree/master/lib/binding_web) |
| Go | Official `tree-sitter/go-tree-sitter` (MIT, 299 stars, last push 2025-11-16, cgo + optional purego loading); `smacker/go-tree-sitter` (571 stars, ~60 grammars bundled, last push 2024-08-27) | Works but **cgo**; Vale notes building from source needs a C compiler "because the source-code parsers are built with cgo" | [official](https://github.com/tree-sitter/go-tree-sitter), [official API](https://api.github.com/repos/tree-sitter/go-tree-sitter), [smacker](https://github.com/smacker/go-tree-sitter), [smacker API](https://api.github.com/repos/smacker/go-tree-sitter), [Vale install](https://docs.vale.sh/topics/installation) |

Grammars needed for an MVP symbol index: TypeScript/JavaScript, Python, Go, Rust, Java, plus `bash`. Each needs a hand-written `tags`/query file; tree-sitter grammars are individually MIT repos under the tree-sitter org.
Alternative: `ast-grep` already ships 27 built-in languages and is MIT/Rust; its CLI can be shelled out to for structural queries (`--json`, `--format sarif`) ([languages](https://ast-grep.github.io/reference/languages.html), [scan CLI](https://ast-grep.github.io/reference/cli/scan.html)). Depending on a second binary is a distribution cost, but it avoids writing per-language queries from scratch.

Markdown parsing with tree-sitter is **not** recommended: tree-sitter-markdown says it has "known inaccuracies" and is not recommended where correctness is critical, and WASM use needs extra work ([repo](https://github.com/tree-sitter-grammars/tree-sitter-markdown)). Use a CommonMark parser for docs and tree-sitter only for source.

### B3. Docstring vs signature
- **pydoclint** (Python): fast, 39 rules, numpy/google/sphinx styles, baseline mode, flake8 plugin; MIT, active ([README](https://github.com/jsh9/pydoclint), [API](https://api.github.com/repos/jsh9/pydoclint)).
- **darglint**: **archived** (last push 2022-12-08) ([API](https://api.github.com/repos/terrencepreilly/darglint)). Don't depend on it.
- **eslint-plugin-jsdoc** (JS/TS): `check-param-names`, `require-param`, `check-types`; TS/TSDoc configs ([README](https://github.com/gajus/eslint-plugin-jsdoc)). License shows as NOASSERTION in the API ([API](https://api.github.com/repos/gajus/eslint-plugin-jsdoc)); check it before bundling.
- Strategy for a multi-language CLI: **orchestrate** these (detect language, call existing linter, normalize to our JSON/SARIF) instead of reimplementing. Go/Rust have `go vet`/`rustdoc` lints (not researched here **[unverified]**).

### False-positive risks
- Documented names that are conceptual, not literal symbols (prose mentioning `User`), overloads, decorators/DI that rename routes, prefix routers (`/api/v1` mount points), generated clients. Version-specific docs (old API deliberately documented). Route params in different syntaxes (`:id`, `{id}`, `<int:id>`).

### Effort
- OpenAPI spec-vs-generated-spec wrapper around oasdiff: **S-M**.
- Markdown-to-symbol existence check for 3 languages via tree-sitter: **L**.
- Route-literal cross-check across frameworks: **L-XL** (long tail).
- Docstring orchestration: **M**.

---

## 3. Pain C: specs/ADRs that contradict code

### What is realistically deterministic
1. **Dangling references**: ADR/spec mentions a path (`path/to/billing/`), file, module, symbol or link that no longer exists -> `stat()` plus symbol index (same machinery as A/B). Highest precision of the C family.
2. **Dependency claims**: ADR text like "we use Redis/Prisma/Kafka" cross-checked against manifests (`package.json`, `go.mod`, `Cargo.toml`, `pyproject.toml`, `requirements.txt`). Needs a curated alias list (e.g. "Postgres" -> `pg`, `psycopg`, `sqlx`) and works best when ADRs mark decisions in a machine-readable way (front-matter `depends-on:` / `uses:`). Free text is NLP territory -> noisy. If a dependency is **absent** from manifests while ADR status is `accepted`, that is a finding; the inverse (new dep with no ADR) is a policy lint.
3. **Status consistency**: ADR statuses (`proposed|accepted|deprecated|superseded`) vs links; "superseded by 0012" where 0012 does not exist. MADR defines the format ([adr/madr](https://github.com/adr/madr), license shows NOASSERTION in [API](https://api.github.com/repos/adr/madr)). Log4brains treats ADR status as the only mutable field and has `@adr` code annotations listed as "Coming soon" ([Log4brains](https://github.com/thomvaill/log4brains)).
4. **Git-history heuristic (staleness, not contradiction)**: for each doc, compute `last_doc_touch` and count commits / changed lines in the "code area" it references (paths it mentions or CODEOWNERS-style mapping) since then; warn above a threshold (e.g. N commits or X% churn). Pure `git log`; cheap; a **signal**, not proof. Must be reported as a lower-severity "warn" class.

### Existing tools
- **adr-tools** (shell, 5.7k stars, last push 2024-04-25): scaffolding only (`adr init`, `adr new`, supersede with `-s`); no validation of code references ([README](https://github.com/npryce/adr-tools), [API](https://api.github.com/repos/npryce/adr-tools)).
- **log4brains** (TS, Apache-2.0, last push 2024-12-17): preview/publish site; no built-in CI checks; uses git logs for metadata ([README](https://github.com/thomvaill/log4brains), [API](https://api.github.com/repos/thomvaill/log4brains)). Both are creation/publishing tools, not drift checkers, so **no incumbent** in this deterministic niche.

### The arXiv paper 2603.00489, checked against the primary source
"Does My README File Need To Be Updated? Exploring LLM-Based README Maintenance" (Gao, Lin, Treude, Gay, Zahedi; submitted 2026-02-28, v2 2026-08-21) ([abs](https://arxiv.org/abs/2603.00489), [HTML](https://arxiv.org/html/2603.00489)). Corrections to the premise that it is a "heuristic layer":
- It is **LLM-based, per pull request**: inputs are PR title, description, commit messages, changed file names, optionally raw patches, and the README split into paragraphs; evaluated on 25,511 PRs from 714 repos. Best configuration recovers about half of PRs historically accompanied by README edits, with 28% user-facing accuracy under real prevalence.
- Its only deterministic component (C3) is a retrieval step ranking patches via MiniLM embedding cosine similarity, which is neither rule-based nor model-free.
- Useful lessons for us: raw patches add noise and cut specificity (Gemma 90.0% -> 50.9%); file names plus description/commits work best; "update needed?" precision is poor (28%). Conclusion: a **path-based trigger** ("PR touches files under X, README sections reference X, README not touched") is the only deterministic analogue, and it should be advisory, not a blocking finding.
- The paper is the right citation for "why we do not block CI on a README-needs-update heuristic".

### False-positive risks
High for free-text (aliases, renamed concepts), moderate for churn heuristics (refactors, formatting commits, vendored dirs), low for dangling references.

### Effort
- Dangling path/symbol references in ADR/spec dirs: **S** (reuses A).
- Manifest-dependency claims with front-matter convention: **M**; free-text: **XL** and not recommended.
- Git churn staleness: **S-M**.

---

## 4. Pain D: broken onboarding guides

### Deterministic check design
1. **Env vars**: extract names the doc mentions (`export FOO=...`, `` `FOO` `` in setup sections, `.env` snippets) and compare with:
   - `.env.example` / `.env.sample` keys (parse dotenv). `dotenv-linter diff .env .env.example` already reports missing keys between files but has no JSON output and only compares env files, not docs/code ([README](https://github.com/dotenv-linter/dotenv-linter), MIT, last push 2026-02-05 per [API](https://api.github.com/repos/dotenv-linter/dotenv-linter)).
   - Env reads in code: `process.env.X` / `process.env['X']` (Node), `os.environ["X"]` / `os.getenv("X")` (Python), `os.Getenv("X")` (Go), `std::env::var("X")` (Rust). Extract with tree-sitter/ast-grep queries or conservative regexes. Dynamic keys (`process.env[name]`) are unresolvable and must be ignored silently.
   - Three-way report: read-by-code but undocumented; documented but never read; in `.env.example` but undocumented. Precision is good for literal names.
2. **Tool versions**: documented "Node 18+", "Go 1.21", "Python 3.11" vs `.nvmrc`, `.node-version`, `.tool-versions` (asdf format: one `tool version` per line, `#` comments, fallbacks) ([asdf docs](https://asdf-vm.com/manage/configuration.html)), `go.mod` `go` directive, `package.json` `engines` (note: advisory only unless `engine-strict`) ([npm docs](https://docs.npmjs.com/cli/v10/configuring-npm/package-json#engines)), `rust-toolchain.toml`, `.python-version`, `pyproject` `requires-python`. Compare with semver-range logic; flag when the doc's minimum is below/above the machine-readable one. Regex over setup sections of docs for `<tool> <version-ish>`; must handle "18+", "v20", ">=3.10".
3. **Ports**: documented `localhost:3000` vs `EXPOSE`/`ports:` in Dockerfile/compose, `PORT` defaults in code, framework config (`vite.config`, `application.yml`). Per-framework and fuzzy; treat as L and optional.
4. **Referenced paths/commands**: reuse pain A engine over "Getting started"/"Setup"/"CONTRIBUTING" docs.
5. **Prerequisite steps**: "run `npm install` before `npm run dev`": ordering checks are heuristic; skip in MVP.

### False-positive risks
Env vars set by the platform (`CI`, `HOME`, `PATH`, `NODE_ENV`) need an allowlist; env vars read only in tests/optional features; docs listing versions of *other* tools; monorepos with several `.nvmrc`; version ranges vs exact pins; Docker-only setups where versions live in Dockerfile `FROM` lines (add as a source).

### Effort
- Env var three-way check (3 languages, literals only): **M**.
- Tool-version consistency (Node/Go/Python/Rust sources): **M**.
- Ports: **L** (long tail). Full onboarding execution (spin up a container and follow steps): **XL** and not deterministic enough.

---

## 5. Stack decision

### How comparable OSS CI linters ship (pattern confirmed)
| Tool | Language | Channels found | Machine output | Source |
|---|---|---|---|---|
| lychee | Rust | prebuilt binaries, cargo/binstall, Docker Hub, brew/scoop/winget/choco/etc., GitHub Action (`lychee-action`), pre-commit; npm/pip not listed | compact, detailed, json, junit, markdown; exit codes 0/1/2/3; no SARIF listed | [README](https://github.com/lycheeverse/lychee), [pre-commit file](https://raw.githubusercontent.com/lycheeverse/lychee/HEAD/.pre-commit-hooks.yaml) |
| Vale | Go | binaries, brew/choco/etc., npm/PyPI/mise pins, Docker `jdkato/vale`, GitHub Action `vale-cli/vale-action`, pre-commit; `go install` needs a C compiler (cgo) | n/a checked | [install docs](https://docs.vale.sh/topics/installation) |
| ast-grep | Rust | npm (`@ast-grep/cli`, platform binaries via optionalDependencies), pip `ast-grep-cli`, cargo, brew, macports | `--format github|sarif`, `--json`; exit 1 on error-severity match | [quick start](https://ast-grep.github.io/guide/quick-start.html), [npm](https://registry.npmjs.org/@ast-grep/cli/latest), [scan ref](https://ast-grep.github.io/reference/cli/scan.html) |
| typos | Rust | binaries, cargo, brew, conda, pacman, GitHub Action, pre-commit; `--format json`; exit 2 on typos | JSON lines | [README](https://github.com/crate-ci/typos) |
| actionlint | Go | binaries, brew, `go install`, Docker, download script, GitHub Actions, pre-commit | text on the page I fetched; templates not confirmed | [repo](https://github.com/rhysd/actionlint), [Docker/pre-commit files](https://raw.githubusercontent.com/rhysd/actionlint/HEAD/.pre-commit-hooks.yaml) |
| markdownlint-cli2 / -cli | Node | npm; pre-commit hook; `-cli` also has a Dockerfile | n/a checked | [API](https://api.github.com/repos/DavidAnson/markdownlint-cli2) |
| semgrep | OCaml/Python | pip, brew, Docker; pre-commit hook present | SARIF supported (not re-verified here) | [API](https://api.github.com/repos/semgrep/semgrep) |

**Confirmed pattern**: single binary on GitHub Releases + `.pre-commit-hooks.yaml` + Docker image + GitHub Action (usually in a separate `*-action` repo or an `action.yml` at repo root: typos has it at the root, lychee/Vale use separate repos) + package-manager taps. For Rust CLIs, an npm/pip wrapper that ships per-platform binaries is a recognised extension (ast-grep does both). `cargo-binstall` consumes GitHub release artifacts using `Cargo.toml` metadata and falls back to `cargo install` ([repo](https://github.com/cargo-bins/cargo-binstall)). pre-commit can bootstrap Go, Rust, Node, Python and Ruby hooks without system tools; Docker hooks need a running engine ([pre-commit docs](https://pre-commit.com/#supported-languages)).

### Comparison
| Criterion | Go | Rust | TypeScript/Node | Python |
|---|---|---|---|---|
| Single static binary | Yes (pure-Go deps); **cgo** needed for tree-sitter, hurting cross-compile (Vale documents the C compiler requirement) | Yes; tree-sitter is first-party and native | No (bundle with pkg/bun/SEA = extra complexity **[unverified]**) | No; `uvx` gives zero-install isolated runs but needs uv/Python ([uv docs](https://docs.astral.sh/uv/guides/tools/)) |
| Markdown AST with positions | goldmark (v1.8.x `Node.Pos()`/`Lines()`; v2 branch gives all nodes start positions) | markdown-rs (mdast + positions, 100% CommonMark/GFM claims) or pulldown-cmark offset iterator | remark/mdast (unist positions with line/col/offset) - best ecosystem | markdown-it-py: block tokens have line `map` only; mistune: no position info found |
| Shell parser | `mvdan/sh` (best-in-class, pure Go) | tree-sitter-bash | `bash-parser`/tree-sitter-bash (WASM) **[unverified]** | `bashlex`/tree-sitter **[unverified]** |
| tree-sitter | Official binding (299 stars, last push 2025-11-16) + smacker (stale since 2024-08) | Best | native or WASM (slow in Node) | Healthy |
| Install in arbitrary CI | Download binary, brew, Docker, `go install` | Same + `cargo binstall` + npm wrapper | `npx` (needs Node; most CIs have it) | `uvx`/pipx |
| Dev speed for MVP | High | Medium (borrow checker, but great crates) | Highest for markdown/text work | High |
| Contributor pool for a docs/CI tool | Large | Medium | Largest | Large |
| Real-world precedent | Vale, actionlint, oasdiff, driftdoc | lychee, typos, ast-grep | markdownlint, stalecheck | semgrep, pydoclint |

Markdown parser sources: [goldmark](https://github.com/yuin/goldmark), [goldmark ast docs](https://pkg.go.dev/github.com/yuin/goldmark/ast), [pulldown-cmark](https://github.com/pulldown-cmark/pulldown-cmark), [markdown-rs](https://github.com/wooorm/markdown-rs), [unist](https://github.com/syntax-tree/unist), [mdast](https://github.com/syntax-tree/mdast), [remark](https://github.com/remarkjs/remark), [markdown-it-py](https://markdown-it-py.readthedocs.io/en/latest/using.html), [mistune](https://github.com/lepture/mistune). `code` nodes in mdast expose `lang` and `meta`, which is exactly what fenced-block extraction needs.

### Recommendation: **Go**, pure-Go MVP, defer tree-sitter
Reasoning (my judgement, grounded in the sources above):
1. The MVP I recommend (pain A + slices of D) needs **markdown AST + shell tokenizer + manifest parsers + stat()**, not tree-sitter. Go has the best pieces for exactly that, all pure Go: goldmark (positions) and `mvdan/sh`. This keeps the binary static and cross-compilation trivial (goreleaser-style), dodging the cgo cost.
2. A single static binary is the dominant, proven distribution mode (lychee, Vale, actionlint, typos). Go yields it with the least friction; add Docker, a composite GitHub Action, a `.pre-commit-hooks.yaml` (language `golang` is bootstrapped by pre-commit), and later an npm/pip wrapper that downloads the binary (same trick as ast-grep).
3. When pains B/D need code parsing, either shell out to `ast-grep --json` for structural queries (27 languages built in) or enable the official `go-tree-sitter` behind a build tag. Avoid `smacker/go-tree-sitter` (stale since 2024-08-27).
4. **Runner-up: Rust.** Pick it if tree-sitter-heavy work becomes the core (B2) or if you want one codebase that is also `cargo binstall`-able and wrapped on npm like ast-grep. Cost: slower MVP iteration.
5. **TypeScript** is the best choice only if you want the fastest markdown plumbing (remark ecosystem) and accept a Node dependency on every CI; it is also where `stalecheck` already lives, so you would be competing in its home turf.
6. **Python**: viable via `uvx`, but a multi-language-repo CLI that must "just run in any CI" should not need an interpreter.

### SARIF and annotations
- SARIF 2.1.0 is an OASIS Standard (with Errata 01, 2023-08-28) ([spec](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)).
- GitHub code scanning accepts **only 2.1.0**; required properties include `$schema`, `version`, `runs`, `tool.driver.name`, rule `id`/`shortDescription`/`fullDescription`/`help`, result `message.text`, `locations` and `partialFingerprints`. Limits: 10 MB gzipped per file, 20 runs per file, 25,000 results/rules per run (top 5,000 displayed). `ruleId` and file paths must stay stable across runs to avoid duplicate alerts; PR annotations appear only for lines inside the PR diff ([SARIF support](https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/sarif-support-for-code-scanning)).
- Upload with `github/codeql-action/upload-sarif@v4`, `security-events: write`; the action computes missing `partialFingerprints`. **Private/internal repos need GitHub Code Security / Advanced Security** ([upload docs](https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/uploading-a-sarif-file-to-github)). Therefore also support the zero-license alternative: workflow commands `::error file=...,line=...,col=...,title=...::msg` create annotations ([workflow commands](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands)).
- Emission precedent: ast-grep `--format sarif`/`github`, stalecheck `--sarif` ([scan ref](https://ast-grep.github.io/reference/cli/scan.html), [stalecheck](https://github.com/Allan-Nava/stalecheck)). lychee does not list SARIF ([README](https://github.com/lycheeverse/lychee)). Design note: use stable rule IDs (`DD-A001`...) and our own fingerprint (check + doc + subject), the same keying that stalecheck uses for baselines.
- Suggested exit codes (convention from lychee/typos): 0 clean, 1 findings, 2 usage/config/IO error (lychee uses 2 for findings and 3 for config; typos uses 2 for findings, so there is no universal standard) ([lychee](https://github.com/lycheeverse/lychee), [typos](https://github.com/crate-ci/typos)).

### Optional LLM layer (brief)
- Keep core deterministic and offline; LLM is a separate opt-in subcommand/flag that only **triages or explains** deterministic findings, and never creates blocking findings.
- Pluggability pattern: talk to an OpenAI-compatible Chat Completions endpoint with configurable `base_url`/model/key; this covers OpenAI, many gateways, and Ollama, which exposes `/v1/chat/completions` at `http://localhost:11434/v1/` with a dummy API key ([Ollama docs](https://docs.ollama.com/api/openai-compatibility)). Add a thin native adapter for Anthropic if you need its features. Constrain output to a JSON schema, temperature 0 (the arXiv paper used temperature 0 and constrained single-token decisions) ([paper HTML](https://arxiv.org/html/2603.00489)).
- Warn about precision: the paper's best README-update recommender reached only 28% user-facing accuracy ([abs](https://arxiv.org/abs/2603.00489)).

---

## 6. Name availability (checked 2026-10-10)

Registry checks by direct HTTP: `registry.npmjs.org/<n>`, `pypi.org/pypi/<n>/json`, `crates.io/api/v1/crates/<n>`, GitHub search API (`/search/repositories?q=<n>+in:name`), `pkg.go.dev/github.com/<n>/<n>`. Domain checks used `rdap.org` (200 = registered, 404 = no record); RDAP can be rate-limited and wrong, so confirm at a registrar. GitHub totals are repo-name matches, not exact.

| Name | npm | PyPI | crates.io | Go path (`github.com/<n>/<n>`) | GitHub repos (name match) | Domains (.com/.dev/.io) | Verdict |
|---|---|---|---|---|---|---|---|
| docdrift | **Taken** (v0.0.3, "Detect when code drifts from documentation") ([npm](https://registry.npmjs.org/docdrift)) | **Taken** (v2.1.0, "Detect and fix stale documentation...") ([PyPI](https://pypi.org/pypi/docdrift/json)) | free | free | 47 repos mention it, all about 0-1 stars, incl. `tomdevtech/docdrift`, `JimmyLi2468/DocDrift` ([search](https://api.github.com/search/repositories?q=docdrift+in:name,description&sort=stars)) | registered / registered / not registered | **Collides**, same concept, crowded |
| driftdoc | free | free | free | free | 8 repos; `thesatellite-ai/driftdoc` is a Go doc-drift engine (same concept) ([search](https://api.github.com/search/repositories?q=driftdoc+in:name,description)) | registered / registered / not registered | Package registries free, but **conceptual collision** with a same-category Go tool (tiny, 0 stars) |
| stalecheck | free | free | free | free | 7 repos; `Allan-Nava/stalecheck` is a deterministic doc-drift tool (created 2026-09-28) ([repo](https://github.com/Allan-Nava/stalecheck)); `cemen2009/stalecheck` is a dependency age auditor | registered (.com) / not registered (.dev, .io) | Registries free, **direct competitor uses the name on GitHub** (unpublished on npm under that bare name; it is scoped `@allan_nava/stalecheck`) |
| doclint | free (but `@onshape/doclint`, `@haiy1/doclint` exist) ([npm search](https://registry.npmjs.org/-/v1/search?text=doclint&size=5)) | free | free | free | 32 repos, e.g. `friendly-project/doclint` (2 stars) | registered (.com) / not registered (.dev) | Technically free, but generic and crowded in search; weak branding |
| docsync | free | **Taken** ("Google Docs tabs to Markdown CLI sync tool") ([PyPI](https://pypi.org/pypi/docsync/json)) | free | free | 290 repos (`CoryG89/docsync` 27 stars) | registered .com / .dev; .io not registered | **Collides** and implies "sync", not "check" |

Practical reading: all five are either taken somewhere or conceptually crowded. Cleanest on registries are `driftdoc` and `stalecheck`, but each has a same-category GitHub project. If you want a defensible name, consider a less descriptive coined one and re-run the checks above (npm, PyPI, crates, GitHub, `.dev`) before committing.

---

## 7. Cross-cutting risks and recommendations for the product
- Precision beats recall. A CI gate that cries wolf gets deleted. Ship: severity levels, `ignore` comments, per-check enable flags, and `--baseline` so teams adopt without fixing history (stalecheck's approach).
- Report with exact `file:line:col`: pick a parser that keeps positions (goldmark, markdown-rs, remark) and keep the original byte offsets for fenced-block content so a finding inside a code block maps to its source line.
- Static only by default; execution (`--exec`) is opt-in and sandboxed, since Doc Detective and byexample are execution-based and carry AGPL/GPL licenses ([Doc Detective](https://github.com/doc-detective/doc-detective), [byexample](https://github.com/byexamples/byexample)).
- Do not depend on archived projects: Optic, swagger-cli, darglint (see section 2).
- Licensing: AGPL (Doc Detective) and GPL (byexample, shellcheck) are not embeddable in a permissively licensed CLI; goldmark (MIT), mvdan/sh (BSD-3), oasdiff (Apache-2.0) are ([goldmark](https://github.com/yuin/goldmark), [mvdan/sh API](https://api.github.com/repos/mvdan/sh), [oasdiff API](https://api.github.com/repos/oasdiff/oasdiff), [shellcheck API](https://api.github.com/repos/koalaman/shellcheck)).

---

## 8. Executive summary (10 lines)
1. Every pain has a useful deterministic core; only B (code vs docs) and free-text C are a long tail needing per-language parsers.
2. Pick **Pain A (README commands and paths that no longer resolve)** for the MVP: highest frequency, high precision, and effort **M** because it needs no tree-sitter.
3. Add the cheap slice of D (env vars vs `.env.example`, tool versions vs `.nvmrc`/`go.mod`/`engines`) in v0.2, effort **M**; dangling-reference checks for ADRs (C) come almost free from the same engine.
4. Defer B (symbol/route drift, **L-XL**) and heuristic git-churn warnings (advisory only); the arXiv paper 2603.00489 is LLM-based (28% accuracy), not a deterministic heuristic.
5. Wrap, don't rebuild: oasdiff for spec-vs-spec, pydoclint and eslint-plugin-jsdoc for docstrings; avoid Optic, swagger-cli, darglint (all archived).
6. Stack: **Go**, pure-Go (goldmark + mvdan/sh), single static binary, no cgo until tree-sitter is needed; runner-up Rust (lychee/ast-grep model).
7. Ship as GitHub Releases binary + Docker + GitHub Action + pre-commit hook, plus later an npm/pip binary wrapper; this is the confirmed pattern of lychee, Vale, typos, actionlint, ast-grep.
8. Output text + JSON + SARIF 2.1.0 (GitHub supports only 2.1.0; private repos need Code Security, so also emit `::error` annotations); exit 0 clean, 1 findings, 2 error.
9. Competitors exist but are immature: `stalecheck` (JS, npm-run only) and `driftdoc` (Go, grep claims); your edge is language-agnostic runners + SARIF + baselines.
10. Names: all five candidates are taken or conceptually crowded (docdrift, docsync taken on package registries; driftdoc and stalecheck collide with same-category repos; doclint is generic); choose and re-verify a new coined name.
