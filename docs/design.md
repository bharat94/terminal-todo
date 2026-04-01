# Design

## Storage

### Location

`<project>/.terminal-todo/tasks.bin`

### Format: Binary (MessagePack)

Primary storage is binary (MessagePack) for minimal disk footprint.

**Schema:**
```go
type Task struct {
    ID        uint64    // Auto-increment integer
    Title     string    // Task title/description
    Status    uint8     // 0=pending, 1=in_progress, 2=completed
    Depends   []uint64 // Task IDs this task depends on
    Created   uint64    // Unix timestamp milliseconds
    Completed uint64    // Unix timestamp milliseconds (0 if not complete)
}
```

**File Structure:**
```
tasks.bin: [Task count (varint)] [Task 1] [Task 2] ... [Task N]
```

- All tasks serialized with MessagePack
- Single file for simplicity

### Optional JSON Export

```bash
todo export                    # Export to tasks.json (minified)
todo export --pretty          # Export to tasks.json (formatted)
```

---

## CLI Commands

### Initialization

```bash
todo init
```

Creates `.terminal-todo/` directory and `tasks.bin` (empty).

### Task Management

| Command | Description |
|---------|-------------|
| `todo add "<title>"` | Add task with no dependencies |
| `todo add "<title>" --after <id>` | Add task depending on another |
| `todo add "<title>" --after <id> --after <id2>` | Add task with multiple deps |
| `todo done <id>` | Mark task complete |
| `todo rm <id>` | Remove task |
| `todo cat <id>` | Show task details |

### Dependency Queries

| Command | Description |
|---------|-------------|
| `todo depends <id>` | Show what this task depends on |
| `todo dependents <id>` | Show tasks that depend on this |
| `todo next` | Show tasks ready to work (all deps satisfied) |

### Status & Info

| Command | Description |
|---------|-------------|
| `todo status` | Show all tasks with status |
| `todo status --json` | JSON output for scripting |
| `todo export` | Export to JSON |
| `todo export --markdown` | Export to Markdown |

### Maintenance

| Command | Description |
|---------|-------------|
| `todo prune` | Remove all completed tasks |

---

## Output Formats

### `todo status`

```
ID    STATUS       TITLE              DEPENDS
1     [x]          Research auth      -
2     [ ]          Fix auth bug       1
3     [ ]          Write tests        2
```

### `todo next`

```
Ready to work:
- ID 1: Research auth (no dependencies)
- ID 2: Fix auth bug (depends on: 1 [x])
```

### `todo status --json`

```json
{
  "tasks": [
    {"id":1,"title":"Research auth","status":"completed","depends":[]},
    {"id":2,"title":"Fix auth bug","status":"pending","depends":[1]},
    {"id":3,"title":"Write tests","status":"pending","depends":[2]}
  ]
}
```

### `todo next --json`

```json
{
  "ready": [
    {"id":1,"title":"Research auth"},
    {"id":2,"title":"Fix auth bug","blocked_by":[1]}
  ]
}
```

### `todo export --markdown`

```markdown
# Project Tasks

## Completed
- [x] ID 1: Research auth

## Pending
- [ ] ID 2: Fix auth bug (blocked by: 1 - Research auth)
- [ ] ID 3: Write tests (blocked by: 2 - Fix auth bug)
```

---

## DAG Logic

### Cycle Detection

On `add --after`:
1. Build adjacency list from existing tasks + new dependency
2. Run DFS to detect cycles
3. Reject if cycle detected with error:
   ```
   Error: adding dependency would create cycle: 1 -> 2 -> 3 -> 1
   ```

### Dependency Resolution

When task is marked done:
1. Find all tasks that depend on this task
2. For each dependent, check if ALL its dependencies are now complete
3. If yes, it's "ready" (shown in `next`)

### Blocked vs Ready

- **Ready:** All dependencies are completed
- **Blocked:** At least one dependency is not completed
- **Independent:** No dependencies

---

## Configuration

### Project Config (Optional)

Location: `.terminal-todo/config.json`

```json
{
  "version": "1.0",
  "created": 1700000000000
}
```

Auto-created on `init`. Version for future migrations.

### Git Integration

Add to `.gitignore`:

```
.terminal-todo/
```

---

## Agent Integration Examples

### Claude Code

```bash
# In a hook or script
NEXT=$(todo next --json)
TASK_ID=$(echo "$NEXT" | jq -r '.ready[0].id')
todo done $TASK_ID
```

### Cursor

Same pattern — invoke CLI from cursor prompts/hooks.

### Shell Wrapper

```bash
# ~/.bashrc or ~/.zshrc
alias tnext='todo next'
alias tadd='todo add'
alias tdone='todo done'
alias tstatus='todo status'
```

---

## Error Handling

| Error | Cause | Resolution |
|-------|-------|------------|
| `Error: not in a project` | No `.terminal-todo/` found | Run `todo init` first |
| `Error: task not found` | Invalid task ID | Check `todo status` |
| `Error: cycle detected` | Dependency would loop | Re-add with different deps |
| `Error: dependency not found` | `--after` references missing ID | Check task IDs |

---

## Implementation Phases

### Phase 1: Core Storage & Basic Commands

- [ ] MessagePack binary storage
- [ ] `todo init`
- [ ] `todo add` (basic, no deps yet)
- [ ] `todo done` (basic)
- [ ] `todo status`
- [ ] `todo cat`

### Phase 2: Dependencies & DAG

- [ ] `todo add --after`
- [ ] Cycle detection (DFS)
- [ ] `todo depends`
- [ ] `todo dependents`
- [ ] `todo next` (ready tasks)
- [ ] Blocked status in output

### Phase 3: Export & Maintenance

- [ ] `todo export --json`
- [ ] `todo export --markdown`
- [ ] `todo rm`
- [ ] `todo prune`

### Phase 4: Polish

- [ ] JSON output flags for all queries
- [ ] Error messages with suggestions
- [ ] Tests (unit + integration)
- [ ] Help text for all commands

---

## Tech Stack

- **Language:** Go 1.21+
- **Serialization:** msgpack (github.com/vmihailenco/msgpack/v2)
- **CLI:** urfav/cli or standard flag
- **No external dependencies** except msgpack

---

## File Structure (Final)

```
.terminal-todo/
├── tasks.bin    # Binary task storage (MessagePack)
└── config.json # Project config (optional)
```

```
terminal-todo/
├── main.go
├── cmd/
│   ├── init.go
│   ├── add.go
│   ├── done.go
│   ├── status.go
│   ├── next.go
│   ├── cat.go
│   ├── rm.go
│   ├── depends.go
│   ├── export.go
│   └── prune.go
├── store/
│   ├── store.go      # Binary read/write
│   └── task.go       # Task struct
├── dag/
│   └── dag.go        # Cycle detection, resolution
├── ui/
│   └── output.go    # Formatted output
└── go.mod
```
