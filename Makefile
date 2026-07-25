SOURCE_FILES?=./...
BIN?=gopssh
MAIN?=./cmd/gopssh
TEST_PATTERN?=.
TEST_OPTIONS?=
OS=$(shell uname -s)
PKG?=./pkg/pssh
GOLANGCI_LINT_VERSION?=v2.11.0

export PATH := ./bin:$(PATH)

# Install all the build and lint dependencies
setup:
	GOBIN=$(CURDIR)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go mod download
.PHONY: setup


# gofmt and goimports all go files
fmt:
	find . -name '*.go' -not -wholename './vendor/*' | while read -r file; do gofmt -w -s "$$file"; goimports -w "$$file"; done
.PHONY: fmt

test:
	go test $(TEST_OPTIONS) -v -race -covermode=atomic -coverprofile=coverage.txt $(SOURCE_FILES) -run $(TEST_PATTERN) -timeout=2m
.PHONY: test

cover: test
	go tool cover -html=coverage.txt
	rm coverage.txt
.PHONY: cover

# Run all the linters
lint:
	./bin/golangci-lint run ./...
.PHONY: lint

# Run all the tests and code checks
ci: build test lint
.PHONY: ci

# Build a beta version of $(BIN)
build: clean $(BIN)
.PHONY: build

clean:
	rm -f $(BIN) coverage.txt
.PHONY: clean

$(BIN):
	go build -o $@ $(MAIN)/main.go

.DEFAULT_GOAL := build
