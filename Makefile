BINARY := todo
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
VERSION := $(shell (git describe --tags --always --dirty 2>/dev/null || echo "dev") | sed 's/^v//')
LDFLAGS := -ldflags="-X main.version=$(VERSION)"

.PHONY: all build test test-race test-short lint clean install release-check release-snapshot

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux .

build-darwin:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-darwin-arm64 .

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
