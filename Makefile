BINARY := todo
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
VERSION := $(shell (git describe --tags --always --dirty 2>/dev/null || echo "dev") | sed 's/^v//')
LDFLAGS := -ldflags="-X github.com/bharat94/terminal-todo/internal/cli.Version=$(VERSION)"

.PHONY: all build test test-race test-short lint clean install release-check release-snapshot

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/todo

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux ./cmd/todo

build-darwin:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-darwin-arm64 ./cmd/todo

test:
	go test ./... -count=1 -timeout 120s

test-race:
	go test ./... -race -count=1 -timeout 120s

test-short:
	go test ./... -short -count=1 -timeout 30s

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...

clean:
	rm -f $(BINARY) $(BINARY)-*

install: build
	install -m 755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)

release-check:
	goreleaser check

release-snapshot:
	goreleaser release --snapshot --clean
