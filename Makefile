GOSRCS := go.mod $(wildcard *.go) $(wildcard */*.go) $(wildcard */*/*.go)

EXEC := geoip

BUILD_TIME ?= $(shell date +'%s')
GIT_HASH ?= $(shell git rev-parse --short HEAD)
GIT_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")

VARS :=
VARS += BuildTime=$(BUILD_TIME)
VARS += GitHash=$(GIT_HASH)
VARS += GitTag=$(GIT_TAG)
LDFLAGS := $(addprefix -X service/version.,$(VARS))

all: $(EXEC)

include .make/golangci-lint.mk
include .make/docker.mk

serve:
	go run . serve

watch: # To install 'nodemon': npm install -g nodemon
	nodemon -e go --signal SIGTERM --exec 'make serve'

tidy: $(PBGO)
	go mod tidy

lint: $(GOLANGCI_LINT)
	$(realpath $(GOLANGCI_LINT)) run

$(EXEC): $(PBGO) $(GOSRCS)
	go mod tidy
	go build -ldflags="$(LDFLAGS)" -o $@

clean: clean/golangci-lint
	rm -f go.sum
	rm -f $(EXEC)

.PHONY: all tidy lint clean test
