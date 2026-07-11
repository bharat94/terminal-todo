# todo: A Simple Task Manager for You and Your AI

`todo` is a tool for your terminal that helps you keep track of what needs to be done in a project. Think of it like a smart, shared sticky-note that lives inside your project folder.

## Why use this?

When you work on code, it’s easy to forget what’s next. It’s even harder when you have **AI agents** (like Claude, Cursor, or ChatGPT) helping you. 

`todo` creates a shared "brain" for your project. You and your AI can both see the same list, add tasks, and mark them as finished. This way, no one does the same work twice!

---

## How to use it (The Basics)

Open your terminal in your project folder and follow these steps:

### 1. Set it up
Run this once to start a new list for your project:
```bash
todo init
```

### 2. Add a task
Add something you need to do:
```bash
todo add "Fix the login button"
```

Tasks can include agent-facing scheduling metadata:
```bash
todo add "Review the locking code" --priority 0.9 --caps go,concurrency
```

### 3. Link tasks together (Dependencies)
Sometimes you can't do one thing until another is finished. You can tell `todo` about this:
```bash
# This says "Write tests" can only happen AFTER task #1 is done
todo add "Write tests" --after 1
```

### 4. Check your progress
See everything on your list:
```bash
todo status
```

### 5. Find out what to do next
If you're not sure where to start, ask for the next ready task:
```bash
todo next
```

### 6. Finish a task
When you're done, mark it off:
```bash
todo done 1
```

For a claimed task, identify the owner when completing or releasing it:
```bash
todo done 1 --as agent-alpha
```

---

## For AI Agents

If you are an AI agent, `todo` is your coordination layer. 
- **Read the graph:** Use `todo status --json` to get a machine-readable view of the project.
- **Find work:** Use `todo next --json` to identify tasks that are "unblocked" and ready for execution.
- **Be a good teammate:** Always `add` your plan before you start and mark tasks as `done` when you finish so other agents know the state of the project.

---

## Simple Commands Reference

| Command | What it does |
| :--- | :--- |
| `todo init` | Starts a new task list here. |
| `todo add "..."` | Adds a task (`--priority 0..1`, `--caps go,testing`). |
| `todo status` | Shows your task list. |
| `todo next` | Shows tasks that are ready to be worked on. |
| `todo lineage <id>` | Shows recursive decomposition and progress. |
| `todo update <id>` | Adds handoff context or changes scheduling metadata. |
| `todo link <alias> <path>` | Registers another project for remote dependencies. |
| `todo done <id>` | Marks a task as finished. |
| `todo rm <id>` | Removes a task entirely. |
| `todo prune` | Cleans up and removes all finished tasks. |

Keep it simple. Get things done.

## Agent handoffs

Agents can leave structured context without rewriting task titles or external
planning files:

```bash
todo update 4 --as agent-alpha \
  --set finding="race occurs during rename" \
  --set file=store/store.go \
  --priority 0.9 --caps go,concurrency
```

## Cross-project dependencies

Link a neighboring initialized project once, then use its alias in task URIs:

```bash
todo link backend ../api-service
todo add "Integrate profile API" --after todo://backend/42
```

`todo next` and lifecycle commands resolve the remote task under a shared store
lock. If the linked project is missing or task 42 is incomplete, the local task
remains blocked.
