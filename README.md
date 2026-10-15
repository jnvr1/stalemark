# stalemark

Catch documentation that no longer matches your code, in CI.

`stalemark` is a single-binary CLI that reads your Markdown (README, onboarding
guides, ADRs) and checks the concrete claims in it against the repository:
commands that no longer exist, paths that were moved, env vars that were
renamed, tool versions that drifted. Deterministic first, zero false positives
as a design goal, non-zero exit code when something is stale.

**Status: pre-alpha.** `stalemark check` runs the v0.1 command checks (npm/pnpm/yarn/bun scripts, `make` targets, `task`, `just`, poetry/uv/pdm/hatch scripts, `go` packages, `cargo` targets) on `README.md`, `CONTRIBUTING.md` and `docs/**/*.md`; path checks and the other milestones are not implemented yet.
See [docs/research](docs/research/) for the landscape and feasibility studies
that shaped the scope.

## Usage

```sh
stalemark check
stalemark check --format text README.md
```

## Planned checks

| Milestone | Check | Compares |
|-----------|-------|----------|
| v0.1 | Commands & paths | README code blocks vs `package.json` scripts, `Makefile`, `Taskfile`, `justfile`, `pyproject`, and referenced files |
| v0.2 | Onboarding | Documented env vars vs `.env.example` and code; documented tool versions vs `.nvmrc`, `.tool-versions`, `go.mod`, `engines` |
| v0.3 | ADR references | Files, modules and dependencies named in ADRs that no longer exist |
| later | API docs | Wraps `oasdiff` and docstring linters instead of rebuilding them |

## Output

Text for humans, JSON for machines, SARIF 2.1.0 for GitHub code scanning.
Exit `0` clean, `1` findings, `2` tool error.

## Distribution (planned)

GitHub Releases binary, Docker image, GitHub Action, pre-commit hook.

## License

MIT
