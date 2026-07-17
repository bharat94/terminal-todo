## Outcome

<!-- What user-visible or operational outcome does this change deliver? -->

## Verification

<!-- List exact tests, platforms, or manual checks performed. -->

- [ ] `gofmt -w .`
- [ ] `go test ./... -race -count=1 -timeout 120s`
- [ ] `go vet ./...`

## Compatibility and risk

<!-- Describe protocol, stored-state, security, concurrency, and rollback impact. -->

- [ ] I updated protocol or compatibility docs if a stable surface changed.
- [ ] I added migration coverage if stored state changed.
- [ ] I used synthetic data and did not commit `.terminal-todo/` state.
