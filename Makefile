BIN := $(HOME)/bin/planty
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse --short HEAD)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build install test

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o planty ./cmd/planty

install:
	go build -trimpath -ldflags "$(LDFLAGS)" -o "$(BIN)" ./cmd/planty

test:
	go test -race ./...
