# Validation gates for jczl-flow. These mirror the checks AGENTS.md requires
# before a Go change is considered complete.

BINARY := bin/jczl-flow

.PHONY: all fmt vet test test-race build clean

all: fmt vet test

# gofmt first, then goimports when installed: explicit normalization before
# import organization. A missing goimports is reported, not fatal.
fmt:
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . \
		|| echo "goimports unavailable: import organization skipped"

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race -v ./...

build:
	go build -o $(BINARY) ./cmd/jczl-flow

clean:
	go clean -testcache
	rm -f $(BINARY)
