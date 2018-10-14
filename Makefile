SOURCE_FILES?=./...
BIN?=gopssh
TEST_PATTERN?=.
TEST_OPTIONS?=
OS=$(shell uname -s)
PKG?=./pkg/pssh

export PATH := ./bin:$(PATH)

# Install all the build and lint dependencies
setup:
	go get -t golang.org/x/tools/cmd/stringer
	go get -t golang.org/x/tools/cmd/cover
	go get -t github.com/pierrre/gotestcover
	go get -t golang.org/x/tools/cmd/cover
	go get -t github.com/caarlos0/bandep
	go get -t github.com/golang/dep/cmd/dep
	go get -t github.com/alecthomas/gometalinter
	dep ensure
	gometalinter --install
	echo "make check" > .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit

ifeq ($(OS), Darwin)
	brew install dep
else
	curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
endif
	dep ensure -vendor-only
	echo "make check" > .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
.PHONY: setup

check:
	bandep --ban github.com/tj/assert
.PHONY: check

# gofmt and goimports all go files
fmt:
	find . -name '*.go' -not -wholename './vendor/*' | while read -r file; do gofmt -w -s "$$file"; goimports -w "$$file"; done
.PHONY: fmt


test:
	go test $(TEST_OPTIONS) -v -race -coverpkg=./cmd/gopssh -covermode=atomic -coverprofile=main_coverage.txt ./cmd/gopssh -run $(TEST_PATTERN) -timeout=2m
	go test $(TEST_OPTIONS) -v -race -coverpkg=$(PKG)      -covermode=atomic -coverprofile=pkg_coverage.txt  $(PKG)      -run $(TEST_PATTERN) -timeout=2m
	cat main_coverage.txt pkg_coverage.txt > coverage.txt
.PHONY: test

cover: test
	go tool cover -html=coverage.txt
	rm coverage.txt
.PHONY: cover

# Run all the linters
lint:
	gometalinter --enable=gofmt --deadline 3m --vendor ./...
.PHONY: lint

# Run all the tests and code checks
ci: build test lint
.PHONY: ci

# Build a beta version of $(BIN)
build: $(BIN)
.PHONY: build

clean:
	rm $(BIN)
.PHONY: clean

$(BIN):
	go build -o $@ cmd/$(BIN)/main.go

## Generate the static documentation
#static:
#	@rm -rf dist/$(BIN).github.io
#	@mkdir -p dist
#	@git clone https://github.com/masahide/masahide.github.io.git dist/masahide.github.io
#	@rm -rf dist/masahide.github.io/theme
#	@static-docs \
#		--in docs \
#		--out dist/.github.io \
#		--title GoReleaser \
#		--subtitle "Deliver Go binaries as fast and easily as possible" \
#		--google UA-106198408-1
#.PHONY: static

# Show to-do items per file.
todo:
	@grep \
		--exclude-dir=vendor \
		--exclude-dir=node_modules \
		--exclude=Makefile \
		--text \
		--color \
		-nRo -E ' TODO:.*|SkipNow|nolint:.*' .
.PHONY: todo


.DEFAULT_GOAL := build
