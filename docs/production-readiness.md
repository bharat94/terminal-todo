# Production readiness

## Verdict

terminal-todo is a candidate for a `v0.1.0-beta.1` public prerelease. It is not
yet a `v1.0.0` maturity claim, and the first tagged workflow must pass before
the release itself can be called production-validated.

The core is designed and tested as production software: state mutations are
serialized and crash-safe within the documented platform boundary, task
allocation is atomic and idempotent, ownership is recoverable, protocols are
versioned, integrations are installable, and releases are reproducible and
attested.

The beta label is appropriate because the project has not yet accumulated
real-world compatibility history across many repositories, agent hosts, and
filesystems. The first release should establish that feedback loop without
pretending the interface can never evolve.

## Evidence

| Area | Evidence | Status |
|------|----------|--------|
| State integrity | Stable sidecar locks, flushed temporary files, atomic replacement, migrations, backups, restore, doctor | Ready |
| Coordination | DAG validation, atomic acquisition, idempotent receipts, leases, heartbeats, retries, recovery events | Ready |
| Protocols | Versioned CLI JSON and JSON-RPC, MCP 2025-06-18 lifecycle, strict parameter decoding, stable errors | Ready |
| Agent integration | Bundled MCP-first skill plus idempotent Codex and Claude installers and checks | Ready |
| Platforms | Native Linux, macOS, and Windows race/build/vet matrix; six release targets | Ready |
| Supply chain | Pinned release tools, snapshot checksums and SPDX SBOMs, configured provenance attestations, reachable-vulnerability scan | Ready for tagged validation |
| Operations | Backup, restore, retention, compaction, compatibility, security, and incident guidance | Ready |
| Open source | MIT license, contribution and support guides, security policy, conduct policy, issue forms, PR template | Ready |
| Repository security | Private vulnerability reporting, vulnerability alerts, Dependabot security updates, secret scanning, push protection | Enabled |
| Distribution | Tag-triggered GoReleaser workflow validated locally with snapshot artifacts; public publishing and OIDC remain unproven | Ready for prerelease validation |

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

## Release gate

Before creating the first tag:

1. Require the green CI workflow on the default branch.
2. Run the release checklist in [releasing.md](releasing.md) from a clean
   checkout.
3. Create the immutable semantic-version tag only as an explicit publication
   decision.
4. Download and verify at least one published archive, checksum, SBOM, binary
   version, and attestation.

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
