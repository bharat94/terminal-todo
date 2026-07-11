#!/bin/bash
# Demo: A Go-capable agent that picks up and completes tasks
set -e

AGENT="go-agent-$$"
echo "Starting $AGENT..."

while true; do
  # Find ready tasks matching our capabilities
  TASK_JSON=$(todo next --capabilities go --json 2>/dev/null)
  TASK_ID=$(echo "$TASK_JSON" | python3 -c "import sys,json; tasks=json.load(sys.stdin).get('available_tasks',[]); print(tasks[0]['id'] if tasks else '')" 2>/dev/null)

  if [ -z "$TASK_ID" ]; then
    echo "$AGENT: No tasks available, waiting..."
    sleep 3
    continue
  fi

  echo "$AGENT: Claiming task $TASK_ID"
  todo claim "$TASK_ID" --as "$AGENT" --ttl 5m

  echo "$AGENT: Working on task $TASK_ID..."
  sleep $(( RANDOM % 3 + 2 ))

  echo "$AGENT: Completing task $TASK_ID"
  todo done "$TASK_ID" --as "$AGENT"

  echo "$AGENT: Task $TASK_ID done!"
done
