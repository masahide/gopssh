SOURCE_FILES?=./...
BIN?=gopssh
MAIN?=./cmd/gopssh
TEST_PATTERN?=.
TEST_OPTIONS?=
OS=$(shell uname -s)
PKG?=./pkg/pssh

export PATH := ./bin:$(PATH)
export GO111MODULE := on

# Install all the build and lint dependencies
setup:
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh
	go mod download
.PHONY: setup


# gofmt and goimports all go files
fmt:
	find . -name '*.go' -not -wholename './vendor/*' | while read -r file; do gofmt -w -s "$$file"; goimports -w "$$file"; done
.PHONY: fmt

test:
	go test $(TEST_OPTIONS) -v -race -coverpkg=$(MAIN) -covermode=atomic -coverprofile=main_coverage.txt $(MAIN) -run $(TEST_PATTERN) -timeout=2m
	go test $(TEST_OPTIONS) -v -race -coverpkg=$(PKG)  -covermode=atomic -coverprofile=pkg_coverage.txt  $(PKG)  -run $(TEST_PATTERN) -timeout=2m
	cat main_coverage.txt pkg_coverage.txt > coverage.txt
.PHONY: test

cover: test
	go tool cover -html=coverage.txt
	rm coverage.txt
.PHONY: cover

# Run all the linters
lint:
	./bin/golangci-lint run --tests=false --enable-all --disable=lll ./...
	#gometalinter --enable=gofmt --deadline 3m --vendor ./...
.PHONY: lint

# Run all the tests and code checks
ci: build test lint
.PHONY: ci

# Build a beta version of $(BIN)
build: clean $(BIN)
.PHONY: build

clean:
	rm -f $(BIN)
.PHONY: clean

$(BIN):
	go build -o $@ $(MAIN)/main.go

.DEFAULT_GOAL := build
