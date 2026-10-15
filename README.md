# stalemark

Catch documentation that no longer matches your code, in CI.

`stalemark` is a single-binary CLI that reads your Markdown (README, onboarding
guides, ADRs) and checks the concrete claims in it against the repository:
commands that no longer exist, paths that were moved, env vars that were
renamed, tool versions that drifted. Deterministic first, zero false positives
as a design goal, non-zero exit code when something is stale.

**Status: pre-alpha.** `stalemark check` runs the v0.1 command checks (npm/pnpm/yarn/bun scripts, `make` targets, `task`, `just`, poetry/uv/pdm/hatch scripts, `go` packages, `cargo` targets), file/directory references (backticked paths and link targets), env vars against `.env.example`, localhost ports against Docker and compose files and tool versions against version files on `README.md`, `CONTRIBUTING.md` and `docs/**/*.md`; the other milestones are not implemented yet.
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
```

How env vars are compared with the code is described in [docs/checks/env.md](docs/checks/env.md).

To adopt stalemark without fixing history, run `stalemark baseline` to record
the current findings. `stalemark check` then skips them, matching on rule,
document and subject (not line numbers), and prints a notice on stderr for
baseline entries that no longer match anything.

## Planned checks

| Milestone | Check | Compares |
|-----------|-------|----------|
| v0.1 | Commands & paths | README code blocks vs `package.json` scripts, `Makefile`, `Taskfile`, `justfile`, `pyproject`, and referenced files |
| v0.2 | Onboarding | Documented env vars vs `.env.example` and code; documented tool versions vs `.nvmrc`, `.tool-versions`, `go.mod`, `engines` |
| v0.3 | ADR references | Files, modules and dependencies named in ADRs that no longer exist |
| later | API docs | Wraps `oasdiff` and docstring linters instead of rebuilding them |

## Output

Text for humans, JSON for machines, SARIF 2.1.0 for GitHub code scanning.
Exit `0` clean (notes do not fail the run), `1` findings, `2` tool error.

## Distribution

Binaries for Linux, macOS and Windows (amd64, arm64) and a Docker image at `ghcr.io/jnvr1/stalemark` are built by the release workflow on `v*` tags.

```sh
docker run --rm -v "$PWD:/work" ghcr.io/jnvr1/stalemark check
```

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
