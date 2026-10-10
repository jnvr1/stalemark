VERSION ?= dev
LDFLAGS := -s -w -X github.com/jnvr1/stalemark/internal/cli.version=$(VERSION)
GOEXE   := $(shell go env GOEXE)

.PHONY: all test lint build fuzz

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
