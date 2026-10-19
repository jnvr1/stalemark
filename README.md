# stalemark

Catch documentation that no longer matches your code, in CI.

`stalemark` is a single-binary CLI that reads your Markdown (README, onboarding
guides, ADRs) and checks the concrete claims in it against the repository:
commands that no longer exist, paths that were moved, env vars that were
renamed, tool versions that drifted. Deterministic first, zero false positives
as a design goal, non-zero exit code when something is stale.

**Status: pre-alpha.** `stalemark check` runs the v0.1 command checks (npm/pnpm/yarn/bun scripts, `make` targets, `task`, `just`, poetry/uv/pdm/hatch scripts, `go` packages, `cargo` targets), file/directory references (backticked paths and link targets), env vars against `.env.example`, localhost ports against Docker and compose files, tool versions against version files, and ADR links, status, module paths and dependency claims on `README.md`, `CONTRIBUTING.md` and `docs/**/*.md`; API docs are not implemented.
See [docs/research](docs/research/) for the landscape and feasibility studies
that shaped the scope.

## Usage

```sh
stalemark check
stalemark check --format text README.md
stalemark check --format json
stalemark check --format sarif > stalemark.sarif
stalemark check --format github
stalemark baseline
```

## Configuration

An optional `.stalemark.yml` in the checked directory (unknown keys are an error):

```yaml
include: [README.md, CONTRIBUTING.md, "docs/**/*.md"] # doublestar globs
exclude: ["docs/research/**"]
disable: [SM-A001]                                    # rule IDs to skip
baseline: .stalemark-baseline.json
env:
  documented-unused: true                             # opt in to SM-E004
adr: {dirs: [docs/adr], historical: warn, freetext: false} # ADR dirs (default: docs/adr, docs/adrs, adr, doc/adr, doc/decisions, docs/decisions); historical ADRs: warn|skip; freetext: opt in to prose claims
```

How env vars are compared with the code is described in [docs/checks/env.md](docs/checks/env.md).

To adopt stalemark without fixing history, run `stalemark baseline` to record
the current findings. `stalemark check` then skips them, matching on rule,
document and subject (not line numbers), and prints a notice on stderr for
baseline entries that no longer match anything.

## API docs

Opt-in checks that wrap external tools ([ADR 0003](docs/adr/0003-external-tool-orchestration.md)).
Without the `api` key of `.stalemark.yml` nothing runs. They run once per directory check.

### pydoclint

Docstrings against Python signatures (rules SM-B010 to SM-B013). Install the tool
(Python 3.10+, minimum version 0.11.1) and make sure it is on PATH; an activated
virtualenv puts it there. A missing or older `pydoclint` is a tool error (exit 2).

```sh
pip install pydoclint
```

```yaml
api:
  pydoclint: {paths: [src], min-version: "0.11.1", severity: warning} # paths default to ["."]
```

stalemark runs it from the checked directory, so the `[tool.pydoclint]` section of
`pyproject.toml` (style, options, baseline) applies. Baseline auto-regeneration is
forced off: stalemark never writes into the repository. Findings default to
`warning`; `severity: note` reports them without failing the run.

### oasdiff

The committed OpenAPI spec against the one your project generates from code
(rules SM-B001 to SM-B003). oasdiff runs as an external tool, not inside
stalemark: install it (minimum version 1.33.0) and make sure it is on PATH. A
missing or older `oasdiff` is a tool error (exit 2).

```sh
go install github.com/oasdiff/oasdiff@latest
```

```yaml
api:
  oasdiff: {spec: openapi.yaml, against: build/openapi.generated.yaml, min-version: "1.33.0", severity: warning}
```

`spec` is the committed file and `against` the generated one, both local files
relative to the checked directory; URLs and git revisions are rejected, so a check
never touches the network. Produce the generated spec in an earlier CI step, before
`stalemark check`; a missing file is a tool error. SM-B001 is a change oasdiff rates
ERR (removed endpoint, retyped parameter, changed response schema), SM-B002 one it
rates WARN, and SM-B003 (always a note) an endpoint the generated spec has and the
committed one lacks. Line and column are reported for YAML specs; JSON specs, and
changes found in a `$ref`-ed file, report line 1. An `.oasdiff.yaml` in the checked
directory is read by oasdiff itself. `go install` builds report no version number
(`oasdiff version main`), so their minimum version is not enforced.

### eslint-plugin-jsdoc

JSDoc against JavaScript signatures (rules SM-B020 to SM-B022: parameter names, missing
`@param`, invalid or undefined types). Needs Node, ESLint 9 or newer and
eslint-plugin-jsdoc 65 or newer, with `eslint` on PATH. For a project-local install,
run stalemark through `npm exec`, which puts `node_modules/.bin` on PATH. A missing or
older tool, or a plugin not found in `node_modules`, is a tool error (exit 2).

```sh
npm install --save-dev eslint eslint-plugin-jsdoc
npm exec -- stalemark check .
```

stalemark runs `eslint --format json` with your own flat config, which must enable the
rules; messages of every other rule are ignored. Minimal `eslint.config.mjs`:

```js
import jsdoc from "eslint-plugin-jsdoc";

export default [
  {
    files: ["**/*.js"],
    plugins: { jsdoc },
    rules: {
      "jsdoc/check-param-names": "error",
      "jsdoc/require-param": "error",
      "jsdoc/check-types": "error",
      "jsdoc/no-undefined-types": "error",
    },
  },
];
```

```yaml
api:
  jsdoc: {paths: [src], min-version: "65.0.0", eslint-min-version: "9.0.0", severity: warning} # paths default to ["."]
```

TypeScript files need the TypeScript parser in your flat config; stalemark does not
configure it. Findings default to `warning`.

## Planned checks

| Milestone | Check | Compares |
|-----------|-------|----------|
| v0.1 | Commands & paths | README code blocks vs `package.json` scripts, `Makefile`, `Taskfile`, `justfile`, `pyproject`, and referenced files |
| v0.2 | Onboarding | Documented env vars vs `.env.example` and code; documented tool versions vs `.nvmrc`, `.tool-versions`, `go.mod`, `engines` |
| v0.3 | ADR references | Files, modules and dependencies named in ADRs that no longer exist |
| later | API docs | Wraps oasdiff, pydoclint and eslint-plugin-jsdoc instead of rebuilding them ([ADR 0003](docs/adr/0003-external-tool-orchestration.md)) |

## Output

Text for humans, JSON for machines, SARIF 2.1.0 for GitHub code scanning.
Exit `0` clean (notes do not fail the run), `1` findings, `2` tool error.

## Distribution

Binaries for Linux, macOS and Windows (amd64, arm64) and a Docker image at `ghcr.io/jnvr1/stalemark` are built by the release workflow on `v*` tags.

```sh
docker run --rm -v "$PWD:/work" ghcr.io/jnvr1/stalemark check
```

### GitHub Action

Installs the release binary, verifies its checksum and runs `stalemark check`. The step fails (exit 1) when there are findings. `@v0` tracks the first release line, so it works once that release is published.

```yaml
- uses: actions/checkout@v7
- uses: jnvr1/stalemark@v0
  with:
    format: github # inline annotations on the PR
```

To surface findings in code scanning, write SARIF to a file and upload it:

```yaml
- uses: jnvr1/stalemark@v0
  continue-on-error: true # exit 1 on findings; the upload below still runs
  with:
    format: sarif
    output: stalemark.sarif
- uses: github/codeql-action/upload-sarif@v4
  if: always()
  with:
    sarif_file: stalemark.sarif
```

Inputs: `version` (default latest), `format` (default `github`), `args` (default `.`), `output`, `working-directory`, `token` (defaults to the workflow token; while this repository is private, a workflow in another repository needs a token that can read its releases).

### Pre-commit

```yaml
repos:
  - repo: https://github.com/jnvr1/stalemark
    rev: <tag>
    hooks:
      - id: stalemark
```

Replace `<tag>` with a release tag once one is published. The hook builds from source with `language: golang` and runs `stalemark check` on the repository root whenever a Markdown file changes.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
