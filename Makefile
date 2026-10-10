VERSION ?= dev
LDFLAGS := -s -w -X github.com/jnvr1/stalemark/internal/cli.version=$(VERSION)
GOEXE   := $(shell go env GOEXE)

.PHONY: all test lint build

all: test lint build

test:
	go test ./...

lint:
	go vet ./...
	golangci-lint run

build: export CGO_ENABLED = 0
build:
	go build -ldflags "$(LDFLAGS)" -o bin/stalemark$(GOEXE) ./cmd/stalemark
