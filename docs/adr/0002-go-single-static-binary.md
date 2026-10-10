# 2. Go, pure Go, single static binary

Date: 2026-10-10

## Status

Accepted

## Context

The tool must run in CI for repositories written in any language, so it cannot
depend on a Node, Python or Rust toolchain being present. See
`docs/research/02-feasibility-and-stack.md` for the full comparison.

## Decision

- Language: Go.
- No cgo. Markdown parsing with `goldmark` (keeps source positions), shell
  parsing with `mvdan.cc/sh`.
- Tree-sitter is deferred until a check needs per-language source parsing
  (API docs milestone).

## Consequences

- One static binary per platform, distributed via GitHub Releases, a Docker
  image, a GitHub Action and a pre-commit hook. Same model as Vale, actionlint
  and lychee.
- Checks that need real source parsing (API docs vs code) are more expensive to
  add later and are explicitly out of the first three milestones.
