VERSION ?= dev
LDFLAGS := -s -w -X github.com/jnvr1/stalemark/internal/cli.version=$(VERSION)
GOEXE   := $(shell go env GOEXE)

.PHONY: all test lint build fuzz fixtures-pydoclint

all: test lint build

test:
	go test ./...

# go test fuzzes one package per invocation, so one line per fuzzer.
fuzz:
	go test ./internal/markdown -run='^$$' -fuzz=FuzzParse -fuzztime=30s
	go test ./internal/shellx -run='^$$' -fuzz=FuzzExtract -fuzztime=30s

lint:
	go vet ./...
	golangci-lint run

build: export CGO_ENABLED = 0
build:
	go build -ldflags "$(LDFLAGS)" -o bin/stalemark$(GOEXE) ./cmd/stalemark

# Re-records the raw pydoclint output next to each fixture (needs pydoclint 0.11.1 on PATH). Stderr only, with
# CRLF and backslashes normalized so the files are identical on every OS.
fixtures-pydoclint:
	for d in testdata/pydoclint/*/; do \
		(cd $$d && pydoclint --show-filenames-in-every-violation-message=True --auto-regenerate-baseline=False --quiet . 2>&1 >/dev/null | tr -d '\r' | tr '\134' '/' > pydoclint.txt; true); \
	done

