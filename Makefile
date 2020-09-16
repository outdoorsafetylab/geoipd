PROTOS := $(wildcard *.proto) $(wildcard */*.proto)
PBGO := $(PROTOS:.proto=.pb.go)

EXEC := geoipd
GOFILES := go.mod $(wildcard *.go) $(wildcard */*.go)

BUILD_TIME ?= $(shell date +'%s')
GIT_HASH ?= $(shell git rev-parse --short HEAD)
GIT_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")

VARS :=
VARS += BuildTime=$(BUILD_TIME)
VARS += GitHash=$(GIT_HASH)
VARS += GitTag=$(GIT_TAG)
LDFLAGS := $(addprefix -X main.,$(VARS))

all: $(EXEC) $(PBJS)

include .make/golangci-lint.mk
include .make/protoc.mk
include .make/protoc-gen-go.mk
include .make/watcher.mk
include .make/backend.mk
include .make/docker.mk

tidy: $(PBGO)
	go mod tidy

lint: $(GOLANGCI_LINT)
	$(realpath $(GOLANGCI_LINT)) run

test:
	go test -count=1 ./test

$(EXEC): $(PBGO) $(GOFILES)
	go mod tidy
	go build -ldflags="$(LDFLAGS)" -o $@

clean/proto:
	rm -f $(PBGO)

clean: clean/golangci-lint clean/protoc clean/protoc-gen-go clean/watcher clean/backend clean/proto
	rm -f go.sum
	rm -f $(EXEC)

.PHONY: all tidy lint clean test stress
