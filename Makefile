.DEFAULT_GOAL := help

BINARY := bin/jczl-flow

.PHONY: build clean fmt help test tidy vet

## Display this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@awk '/^[a-zA-Z0-9_-]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 1, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "  %-15s %s\n", helpCommand, helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

## Build the standalone executable without cgo
build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/jczl-flow

## Remove build artifacts and test cache
clean:
	go clean -testcache
	rm -rf bin/

## Format with gofmt, then organize imports when goimports is installed
fmt:
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . \
		|| echo "goimports unavailable: import organization skipped"

## Run unit tests with race detection and no cache
test:
	go clean -testcache && go test -race -v ./...

## Add missing and remove unused modules
tidy:
	go mod tidy

## Report suspicious constructs
vet:
	go vet ./...
