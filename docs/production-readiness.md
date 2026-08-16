# Production readiness

## Verdict

terminal-todo `v0.1.0-beta.1` is published and production-validated within the
documented beta operating boundary. It is not yet a `v1.0.0` maturity claim.

The core is designed and tested as production software: state mutations are
serialized and crash-safe within the documented platform boundary, task
allocation is atomic and idempotent, ownership is recoverable, protocols are
versioned, integrations are installable, and releases are reproducible and
attested.

The beta label is appropriate because the project has not yet accumulated
real-world compatibility history across many repositories, agent hosts, and
filesystems. The first release establishes that feedback loop without
pretending the interface can never evolve.

The initial opt-in lifecycle smoke run passed against local Codex 0.144.5.
Claude Code 2.1.215 was correctly skipped before model invocation because the
local client was unauthenticated. Since that run, the conformance harness has
grown into an executable nine-scenario, twelve-turn catalog with deterministic
fixtures, multi-turn resume, concurrent contention, authoritative MCP and
persisted-state evidence, and hard-gate scoring. The catalog is fully covered
without model calls in normal CI; a current paid full-catalog host run remains
the next maturity measurement. See [Real-agent conformance](conformance.md)
for the exact boundary.

## Evidence

| Area | Evidence | Status |
|------|----------|--------|
| State integrity | Stable sidecar locks, flushed temporary files, atomic replacement, migrations, bounded persisted input, backups, restore, doctor | Ready |
| Coordination | DAG validation, atomic acquisition, idempotent receipts, leases, heartbeats, retries, recovery events | Ready |
| Protocols | Versioned CLI JSON and JSON-RPC, additive compact receipts and event pages, MCP 2025-06-18 lifecycle and tool annotations, strict parameter decoding, stable errors | Ready |
| Agent integration | Bundled MCP-first skill, bounded session bootstrap, compact routine mutations, allocation diagnostics, idempotent Codex and Claude installers, and an opt-in nine-scenario real-agent conformance runner | Catalog execution is deterministic and CI-tested; Codex lifecycle smoke validated; current full-catalog host certification remains beta evidence |
| Platforms | Native Linux, macOS, and Windows race/build/vet matrix; six release targets | Ready |
| Supply chain | Pinned release tools, checksums, per-archive SPDX SBOMs, provenance attestations, reachable-vulnerability scan | Tagged pipeline and downloaded artifacts validated |
| Operations | Backup, restore, retention, compaction, compatibility, security, and incident guidance | Ready |
| Open source | MIT license, contribution and support guides, security policy, conduct policy, issue forms, PR template | Ready |
| Repository security | Private vulnerability reporting, vulnerability alerts, Dependabot security updates, secret scanning, push protection, protected `master` with strict required checks | Enabled |
| Distribution | Public `v0.1.0-beta.1` release with six platform archives, verified checksums, SBOMs, embedded version, and attestations | Beta validated |

## Verified operating boundary

terminal-todo is a file-backed coordination control plane, not a distributed
database.

Supported production use means:

- all workers see the same project state and stable lock sidecars;
- the filesystem provides the lock and atomic-replace semantics in the
  [compatibility contract](compatibility.md);
- secrets and unnecessary personal data are not written into task metadata;
- backups exist for media loss, deletion, and filesystem corruption;
- workers heartbeat, complete, block, decompose, or release every acquired
  task.

Local Linux, macOS, and Windows filesystems are the supported shape.
Cloud-synchronized directories and ordinary network mounts are best effort
until their semantics are validated in the deployment environment. Copying a
task store between machines is transfer, not live consensus.

## Beta release validation

The first-tag gate was completed for
[`v0.1.0-beta.1`](https://github.com/bharat94/terminal-todo/releases/tag/v0.1.0-beta.1):

1. The tagged commit passed the full Linux, macOS, Windows, release-contract,
   and reachable-vulnerability CI matrix.
2. The release workflow built and published all six configured archives.
3. Every downloaded artifact matched `checksums.txt`; all six SBOMs parsed as
   SPDX 2.3 documents.
4. The native macOS arm64 binary reported `todo v0.1.0-beta.1`, and provenance
   verification passed for all six archives.
5. The `master` branch now requires the five strict CI checks, pull-request
   review flow, conversation resolution, linear history, and administrator
   enforcement; force pushes and deletion are disabled.

## What blocks a 1.0 claim

These are maturity gates, not reasons to delay a beta:

- observed upgrade and migration history across multiple released versions;
- sustained use by independent Codex, Claude, human, and scripted workers;
- fault-injection evidence across more filesystem and power-loss scenarios;
- a documented deprecation window informed by real integrations;
- evidence that diagnostics and onboarding work for users unfamiliar with the
  internal model.

Cross-machine service-backed coordination is a separate future product mode.
It is not required for local and shared-filesystem production readiness, and
it should not be implied by the first release.
