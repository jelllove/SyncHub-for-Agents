# SyncHub versioned specifications

This folder contains repository contracts that are versioned independently of
the prose that explains them.

- [Validation receipt v1](validation-receipt.v1.schema.json) defines the JSON
  written by `node scripts/dev.mjs verify`.
- [Repair proof v1](repair-proof.v1.schema.json) defines the diagnostic repair
  and rollback proof written by `node scripts/dev.mjs repair:verify`.
- [Agentic validation v1](agentic-validation.v1.md) explains how those receipts,
  workflows, labels, and review handoffs fit together.
