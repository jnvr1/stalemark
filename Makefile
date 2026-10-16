VERSION ?= dev
LDFLAGS := -s -w -X github.com/jnvr1/stalemark/internal/cli.version=$(VERSION)
GOEXE   := $(shell go env GOEXE)

.PHONY: all test lint build fuzz fixtures-pydoclint fixtures-oasdiff

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

# Re-records the raw oasdiff JSON next to each fixture (needs oasdiff 1.33.0 on PATH): the exact command the
# wrapper runs, with spec and against read from the fixture's .stalemark.yml. Fixtures whose against file is
# missing are skipped. Named changelog.json because oasdiff reads a file called oasdiff.json as its config.
fixtures-oasdiff:
	for d in testdata/oasdiff/*/; do \
		(cd $$d && s=$$(sed -n 's/^ *spec: *//p' .stalemark.yml) && a=$$(sed -n 's/^ *against: *//p' .stalemark.yml) && [ -f "$$a" ] && oasdiff changelog "$$s" "$$a" --format json | tr -d '\r' > changelog.json; true); \
	done

