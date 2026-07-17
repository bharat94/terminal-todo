# Compatibility contract

terminal-todo's production contract spans operating systems, filesystems,
stored state, and agent protocols. This document separates supported behavior
from best-effort behavior.

## Supported platforms

Release artifacts and CI cover:

| Operating system | Architectures | Lock primitive | CI execution |
|------------------|---------------|----------------|--------------|
| Linux | amd64, arm64 | `flock` | native amd64 build, race tests, CLI/MCP end-to-end tests |
| macOS | amd64, arm64 | `flock` | native arm64 build, race tests, CLI/MCP end-to-end tests |
| Windows | amd64, arm64 | `LockFileEx` | native amd64 build, race tests, CLI/MCP end-to-end tests |

Go 1.26.1 or newer in the Go 1.26 line is required to build from source. The
release pipeline cross-builds all six targets with CGO disabled.

## Filesystem requirements

The coordination store requires a filesystem that provides:

- advisory locks visible to every participating process;
- atomic rename within one directory;
- durable file and directory synchronization;
- coherent reads after rename.

Local APFS, common local Linux filesystems, and local Windows filesystems are
the supported deployment shape. Network mounts, cloud-synchronized folders,
and filesystems with incomplete lock or rename semantics are best effort.
When using one, validate concurrent acquisition and crash recovery under the
actual mount configuration.

All workers coordinating one store must see the same lock sidecar and
`tasks.bin` path. Copying stores between machines is backup or transfer, not
live multi-machine consensus.

## Store compatibility

The MessagePack store carries an integer schema version. A newer binary
automatically migrates supported older schemas while holding the write lock.
A binary refuses to open a store created by a newer unsupported schema rather
than guessing.

Before upgrading an important shared store:

```bash
todo backup
todo --version
```

Roll back the binary only if it supports the current store schema; otherwise
restore the pre-upgrade backup.

## Automation compatibility

Human-readable CLI output is for people and may improve between releases.
Automations should use one of:

- CLI `--json` envelopes with `schema_version`;
- MCP `2025-06-18` through `todo mcp --stdio`;
- the complete native JSON-RPC API through `todo serve --stdio`.

Within protocol version `1`, existing JSON fields, method names, event types,
error identifiers, and numeric JSON-RPC error mappings are append-only.
Clients must ignore unknown object fields. New optional fields, methods,
tools, event types, and error codes may be added without a protocol-version
change. Removing or changing an existing value requires a new protocol
version.

MCP exposes a curated coordination surface. Administrative native methods are
not automatically MCP tools. Tool order is deterministic, but clients should
select tools by name rather than position.

## Verification tiers

Every pull request and push runs:

1. module checksum verification;
2. trimmed builds on Linux, macOS, and Windows;
3. the race-enabled unit, process-concurrency, CLI, MCP, installer, migration,
   backup/restore, and end-to-end suites on all three operating systems;
4. `go vet`;
5. formatting and GoReleaser configuration checks;
6. the Go vulnerability database's reachable-code scan.

Version tags repeat the race suite, build all release architectures, generate
checksums and SBOMs, publish immutable archives, and attach provenance
attestations.

The cross-platform release build proves arm64 compilation. Native arm64
runtime behavior is covered on macOS; Linux and Windows arm64 should be tested
on representative hardware before relying on platform-specific filesystem
behavior at high concurrency.
