BINARY   := umeshctl
PACKAGE  := github.com/opscores/umesh-cli
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PACKAGE)/cmd.Version=$(VERSION) \
	-X $(PACKAGE)/cmd.GitCommit=$(COMMIT) \
	-X $(PACKAGE)/cmd.BuildDate=$(DATE)

.PHONY: all build install test vet lint clean

all: build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY)
