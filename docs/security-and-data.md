# Security and data lifecycle

terminal-todo is designed as a user-owned local coordination control plane.
This document defines its trust boundary, persisted data, retention controls,
and recovery model.

## Trust boundary

The CLI, native JSON-RPC server, and MCP server run with the permissions of the
current OS account. Lease ownership prevents cooperative workers from
accidentally mutating another worker's claimed task; it is not identity
authentication and does not defend against a malicious process that can read
or write the same files.

The MCP server is stdio-only. A configured MCP host can invoke its curated
coordination tools with the same authority as the user running `todo`. Do not
wrap it in an unauthenticated network listener. The native JSON-RPC server
also uses stdio and exposes broader administrative operations such as backup,
restore, configuration, and compaction.

Linked repository paths are explicitly trusted local configuration. A linked
store is read using its own advisory lock, but terminal-todo does not sandbox
the linked path from the current OS user.

## Data stored

`tasks.bin` may contain:

- task titles, dependencies, status, priority, and tags;
- actor names, ownership leases, retry errors, and capabilities;
- structured metadata and human-readable task logs;
- the append-only coordination event stream;
- successful atomic-acquisition receipts, including immutable task snapshots.

`agents.json`, `repositories.json`, and `config.json` contain their respective
registries and project defaults. Backups contain the complete `tasks.bin`
coordination state.

All data is stored in cleartext. Never put passwords, tokens, private keys,
credentials, or unnecessary personal data in titles, errors, logs, actor
names, or metadata. Use a secret manager and record only a reference.

## Local permissions

New stores use restrictive POSIX modes:

| Path | Mode |
|------|------|
| `.terminal-todo/` | `0700` |
| state, registry, backup, and lock files | `0600` |

Existing projects created by older releases may have broader permissions.
Inspect and repair them with:

```bash
todo doctor
todo doctor --fix
```

Permission bits are only one layer. Files copied to a different filesystem,
committed to source control, or synchronized by another tool inherit that
system's access policy. Windows access is governed by the user's ACL and the
filesystem rather than POSIX mode bits.

## Retention

Tasks, logs, events, and acquisition receipts persist until the user removes
them. No background process silently deletes audit or idempotency state.

`todo prune` removes completed tasks and rewrites satisfied local dependency
references. It does not delete the event stream or successful acquisition
receipts.

`todo compact` applies explicit retention policies:

```bash
todo compact --keep-events 10000 --dry-run
todo compact --receipts-before 2160h --dry-run
todo compact --keep-events 10000 --receipts-before 2160h
```

`--keep-events` keeps the newest count while preserving the monotonic next
event ID. Event consumers that request a cursor older than the retained window
receive the oldest retained event and should treat that as a history gap.

`--receipts-before` accepts a Go duration such as `2160h` (90 days).
terminal-todo removes an old receipt only when the corresponding current task
is completed or has been removed. Active task receipts remain protected.
Compacting a receipt ends replay and conflict detection for that request ID;
workers should always generate globally unique opaque IDs rather than relying
on eventual reuse.

Back up before destructive retention when the audit history may be needed.

## Backup and restore

Create a point-in-time snapshot:

```bash
todo backup
todo backup --output /secure/location/project-todo.bin
```

A backup includes tasks, the next task and event sequences, events, and
acquisition receipts. Backup files use mode `0600` when the filesystem
supports POSIX permissions.

Restore replaces the complete coordination store atomically:

```bash
todo restore /secure/location/project-todo.bin
todo doctor
```

Restore does not replace `config.json`, `agents.json`, or
`repositories.json`; back those files up alongside `tasks.bin` when the
entire project configuration must be reconstructed.

Keep backups on storage with appropriate access controls and retention. Test
restores periodically; an untested backup is only a hypothesis.

## Failure and recovery guarantees

Mutations serialize under a stable sidecar lock. Updated state is written to a
private temporary file, flushed, atomically renamed over `tasks.bin`, and
followed by a directory flush where the operating system exposes that
operation. A process failure before rename leaves the previous store intact; a
failure after rename leaves the new complete store. On Windows, a sudden power
loss may leave either complete version because Go cannot explicitly flush the
containing directory.

Expired leases are durably reclaimed and recorded as events. Orphaned
temporary files can be inspected and removed with `todo doctor --fix`.
Filesystem, kernel, and hardware behavior still define the ultimate durability
boundary; use backups for media loss, accidental deletion, or corruption.
