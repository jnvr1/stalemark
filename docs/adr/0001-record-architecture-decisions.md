# 1. Record architecture decisions

Date: 2026-10-10

## Status

Accepted

## Context

stalemark exists to catch documentation drift. Its own decisions must be
written down and kept in sync with the code, or the project contradicts itself.

## Decision

We record every significant architecture decision as an ADR in `docs/adr`,
numbered sequentially, using this format: Status, Context, Decision,
Consequences.

## Consequences

stalemark will run against its own ADRs in CI as soon as the ADR reference
check (v0.3) exists.
