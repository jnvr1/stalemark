# Contributing

stalemark is deterministic and precision-first. Read this before opening a PR.

## Prerequisites

- Go 1.27 (see `go.mod`)
- golangci-lint v2 (the config is `.golangci.yml`)
- make

## Build and test

```sh
make test lint
go test ./...
make build
make fuzz
```

`make test lint` is what CI runs. `make build` writes `bin/stalemark` with
`CGO_ENABLED=0`. `make fuzz` runs the Markdown parser and shell extractor
fuzzers for 30 seconds each.

## How checks are built

Strict TDD. Fixtures come first.

1. Add a case under `testdata/<check>/<case>/`: the Markdown, the files it
   resolves against (`package.json`, `Makefile`, ...) and a hand-written
   `expected.json`.
2. Run `go test ./...` and watch it fail for the right reason.
3. Write the minimum code to make it pass.
4. Run the golden test with `-update` only to confirm it is a no-op. If it
   rewrites `expected.json`, your hand-written golden was wrong or the code is.

One resolver per tool lives under `internal/runner/`. Cover the happy path, the
missing target, and every case where the tool must stay silent (placeholders,
workspace flags, paths created earlier in the block).

## A false positive is a bug

A finding on documentation that is correct is a bug, and it outranks any new
check. Report it with the false-positive issue template and include:

- the exact Markdown snippet
- the repo files it resolves against
- the expected and the actual output

The fix is a fixture that reproduces it plus a narrower resolver. Never silence
a rule to get green.

## Workflow

- Issue first. Branch `type/N-description`, for example `chore/1-scaffold`.
- One PR per issue. The body starts with `Closes #N`.
- Conventional commits: `type(scope): description`.
- CI must be green on Linux, macOS and Windows.
- Keep every command and path in the docs real. The repo checks itself with
  `.stalemark.yml`, so a stale doc fails the build.

## Dogfooding

The `dogfood` CI job builds the binary with `make build` and runs it against
this repository's README, CONTRIBUTING and `docs/`. It also runs it against
`testdata/e2e/drifted` and requires findings there, so the job cannot go green
by checking nothing. Run `go run ./cmd/stalemark check .` locally to get the
same result.

If the job fails on your PR, either the doc is stale (fix it) or the finding is
wrong (open a false-positive issue). A baseline entry with a justification
comment is acceptable only for an intentional example. Never `disable` a rule.

## Stack rules

Read `docs/adr/0002-go-single-static-binary.md` before changing the stack.

- Pure Go, `CGO_ENABLED=0`, one static binary.
- CLI with the stdlib `flag` package and manual dispatch.
- Markdown with goldmark, shell blocks with mvdan.cc/sh.
- Use `filepath`, never `path`, for OS paths. Tests must pass on Windows.

Deliberate shortcuts carry a `ponytail:` comment naming the upgrade path.
