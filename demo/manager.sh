#!/bin/bash
# Demo: Manager agent that creates a multi-step project and watches progress
set -e

PROJECT_ROOT=$(mktemp -d)
cd "$PROJECT_ROOT"

echo "=== Initializing project ==="
todo init

echo ""
echo "=== Creating project: Build Microservice ==="
todo add "Build User Microservice" --priority 0.9 --caps planning
PARENT_ID=1

echo ""
echo "=== Decomposing into sub-tasks ==="
todo decompose "$PARENT_ID" --into '{
  "subtasks": [
    {"title": "Design API schema", "caps": ["go"]},
    {"title": "Implement handlers", "caps": ["go"]},
    {"title": "Write integration tests", "caps": ["go","testing"]},
    {"title": "Add monitoring", "caps": ["devops"]},
    {"title": "QA review", "caps": ["qa"]}
  ]
}'

echo ""
echo "=== Project structure ==="
todo graph
echo ""
echo "(ID 5 has two dependencies below it, making ID 5 depend on IDs 2,3,4)"
todo update 5 --add-dep "todo://local/2" --add-dep "todo://local/3"
todo update 6 --add-dep "todo://local/5"

echo ""
echo "=== Final DAG ==="
todo graph

echo ""
echo "Project initialized at: $PROJECT_ROOT"
echo ""
echo "Open 3 terminals and run:"
echo "  Terminal 1: demo/go-agent.sh"
echo "  Terminal 2: demo/qa-agent.sh"
echo "  Terminal 3: todo watch"
echo ""
echo "Watch as agents automatically pick up and complete tasks!"
