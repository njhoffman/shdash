# shdash task runner. `just` lists the recipes.

set shell := ["bash", "-euo", "pipefail", "-c"]

# Build with the installed Go instead of downloading the go.mod toolchain.
export GOTOOLCHAIN := env("GOTOOLCHAIN", "local")

binary := "shdash"
pkg := "github.com/njhoffman/shdash"
bin_dir := "bin"
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
commit := `git rev-parse --short HEAD 2>/dev/null || echo unknown`
date := `date -u +%Y-%m-%dT%H:%M:%SZ`
ldflags := "-s -w -X " + pkg + "/internal/version.Version=" + version + " -X " + pkg + "/internal/version.Commit=" + commit + " -X " + pkg + "/internal/version.Date=" + date
test_flags := "-race -count=1"

# List the recipes.
default:
    @just --list --unsorted

# Format check, lint, test and build: the pre-commit gate.
check: fmt-check lint test build

# Build bin/shdash.
build:
    mkdir -p {{ bin_dir }}
    go build -trimpath -ldflags '{{ ldflags }}' -o {{ bin_dir }}/{{ binary }} ./cmd/{{ binary }}

# Build, then run shdash with the given arguments.
run *args: build
    ./{{ bin_dir }}/{{ binary }} {{ args }}

# Install shdash into $(go env GOPATH)/bin.
install:
    go install -trimpath -ldflags '{{ ldflags }}' ./cmd/{{ binary }}

# Run the tests (-short skips the one that builds the binary).
test *args:
    go test {{ test_flags }} {{ args }} ./...

# Run one test verbosely: just test-one ./internal/render TestColumns
test-one pkg name:
    go test {{ test_flags }} -run '^{{ name }}$' -v {{ pkg }}

# Run golangci-lint.
lint:
    golangci-lint run

# Format the sources.
fmt:
    gofmt -s -w .

# Fail if any file needs gofmt.
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    out=$(gofmt -s -l .)
    if [ -n "$out" ]; then
      echo "files need gofmt -s -w:" >&2
      echo "$out" >&2
      exit 1
    fi

# Remove build artifacts.
clean:
    rm -rf {{ bin_dir }}
