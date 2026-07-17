# Contributing to terminal-todo

Thanks for helping make terminal-todo a dependable coordination layer. Small,
focused changes with explicit compatibility reasoning are easiest to review.

## Before you start

- Search existing issues before opening a new one.
- Open an issue before a large feature or protocol change so the use case and
  compatibility cost can be discussed first.
- Report vulnerabilities through the private process in [SECURITY.md](SECURITY.md),
  not a public issue.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Development setup

terminal-todo requires Go 1.26.1 or newer.

```bash
git clone https://github.com/bharat94/terminal-todo.git
cd terminal-todo
go mod verify
make build
make test
make test-race
go vet ./...
```

Tests create isolated temporary projects. Do not commit `.terminal-todo/`
state or generated integration files from a local dogfood session.

## Make a change

1. Add or update tests that demonstrate the behavior.
2. Preserve atomic state transitions, ownership checks, DAG validity, and
   stable error semantics.
3. Run:

```bash
gofmt -w .
go test ./... -race -count=1 -timeout 120s
go vet ./...
git diff --check
```

4. For release or packaging changes, also run:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

5. For skill changes, keep `integrations/skills/terminal-todo/SKILL.md`
   concise and run the skill and integration tests.

## Compatibility-sensitive changes

The JSON, JSON-RPC, MCP, stored-state, error-code, and event surfaces are
contracts. Changes to any of these must update
[docs/agent-protocol.md](docs/agent-protocol.md) and include migration or
compatibility tests where applicable.

Do not:

- reuse an existing error identifier for a different condition;
- silently reinterpret an existing field;
- write a store schema that older supported binaries can mistake for their
  own;
- weaken ownership, dependency, or locking invariants for convenience.

See the [compatibility contract](docs/compatibility.md) and
[concurrency model](docs/concurrency-and-locking.md).

## Pull requests

Use a clear title, explain the user-visible outcome, list verification, and
call out protocol or migration impact. Keep unrelated cleanup separate.
GitHub Actions must pass on Linux, macOS, and Windows before merge.

By contributing, you agree that your contribution is licensed under the
project's [MIT License](LICENSE).
