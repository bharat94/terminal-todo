# terminal-todo: Real-World Examples

These examples show how you and your AI agents can use `terminal-todo` to stay organized, from simple daily tasks to complex multi-agent coordination.

---

## Example 1: The "Handoff" (Human + AI)
You start a feature, and an AI agent finished it.

1. **You (Human):** Initialize the project and add a high-level goal.
   ```bash
   todo init
   todo add "Implement User Authentication"
   ```
2. **AI Agent:** Sees the task, breaks it into smaller steps, and starts working.
   ```bash
   # Agent identifies task #1 and breaks it down
   todo decompose 1 --into '{"subtasks": [{"title": "Setup JWT", "caps": ["go"]}, {"title": "Create login endpoint", "caps": ["go"]}]}'
   
   # Agent claims the first subtask
   todo claim 2 --as agent-alpha --ttl 30m
   ```
3. **Outcome:** When the agent finishes task #2, task #3 becomes "Ready" for the next agent (or you!) to pick up.

---

## Example 2: The "Security Guard" (Parallel Agents)
Two agents with different skills work together without bumping into each other.

1. **Security Agent:** Scans the code and finds a bug.
   ```bash
   todo add "Fix SQL Injection in /search" --priority 0.9 --caps "security"
   ```
2. **QA Agent:** Sees the new security task and adds a test that must happen *after* the fix.
   ```bash
   # Add a test that depends on task #4
   todo add "Verify SQL Injection fix with exploit test" --after 4 --caps "qa"
   ```
3. **Outcome:** The QA Agent knows it's "blocked" until the Security Agent marks task #4 as `done`. No manual communication is needed!

---

## Example 3: The "Bridge" (Cross-Repository)
Coordinate work between a Frontend repo and a Backend repo.

1. **Backend Repo (`api-service`):**
   ```bash
   todo add "Create /v1/users/profile endpoint"
   # Task ID is 42
   ```
2. **Frontend Repo (`web-app`):**
   ```bash
   # Link the backend repo so we can see its tasks
   todo link api-service ../api-service
   
   # Add a frontend task that depends on the backend task
   todo add "Build Profile Page" --after todo://api-service/42
   ```
3. **Outcome:** The frontend agent running in `web-app` will see its task as "Blocked" until the backend agent in `api-service` marks task #42 as `done`.

---

## Example 4: The "Feature Branch" Cleanup
Keep your project clean by removing old, finished work.

1. **Manager Agent:** At the end of a sprint, cleans up the board.
   ```bash
   # See what's left
   todo status
   
   # Remove all finished tasks to keep the file small
   todo prune
   ```
2. **Outcome:** The `tasks.bin` file stays tiny (<1KB), and the AI context window isn't cluttered with "completed" noise.

---

## Example 5: High-Priority "Hotfix"
Using priority to jump the queue.

1. **You:** Realize there is a critical bug.
   ```bash
   todo add "URGENT: Fix production crash" --priority 1.0
   ```
2. **Agents:** All agents check `todo next --ready`. Because of the high priority (1.0), this task appears at the very top of their list.
3. **Outcome:** The next available agent claims the hotfix immediately, ignoring lower-priority feature work.
