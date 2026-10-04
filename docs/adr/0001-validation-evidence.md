# ADR 0001: Version repository validation evidence

## Status

Accepted.

## Context

SyncHub now publishes validation, repair, and CI-failure response artifacts from
several workflows. Agents and maintainers need stable contracts for those
artifacts so they can distinguish a complete failed report from a passing check
row or a stale prose claim.

## Decision

Keep versioned receipt contracts in [docs/specs](../specs/README.md), keep the
observable workflow/artifact map in
[agentic observability](../operations/agentic-observability.md), and enforce the
cross-references with `node scripts/dev.mjs docs`.

## Consequences

- Changing report shape, artifact names, labels, or workflow entry points must
  update the matching spec and drift checks in the same pull request.
- The specs improve repository evidence and handoff quality, but they do not
  claim that SyncHub autonomously repairs production incidents or user data.
