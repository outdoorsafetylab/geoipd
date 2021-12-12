PROTOS := $(wildcard *.proto) $(wildcard */*.proto) $(wildcard */*/*.proto)

PBGO := $(PROTOS:.proto=.pb.go)
GOSRCS := go.mod $(wildcard *.go) $(wildcard */*.go) $(wildcard */*/*.go)

EXEC := serviced
DOCKER_IMAGE := geoipd
GEOIP2_LICENSE_KEY ?=

BUILD_TIME ?= $(shell date +'%s')
GIT_HASH ?= $(shell git rev-parse --short HEAD)
GIT_TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")

VARS :=
VARS += BuildTime=$(BUILD_TIME)
VARS += GitHash=$(GIT_HASH)
VARS += GitTag=$(GIT_TAG)
LDFLAGS := $(addprefix -X version.,$(VARS))

all: $(EXEC)

include .make/golangci-lint.mk
include .make/protoc.mk
include .make/protoc-gen-go.mk
include .make/watcher.mk
include .make/docker.mk

watch: $(PBGO) $(WEBINDEX) $(WATCHER) tidy
	$(realpath $(WATCHER)) -c local

tidy: $(PBGO)
	go mod tidy

lint: $(GOLANGCI_LINT)
	$(realpath $(GOLANGCI_LINT)) run

$(EXEC): $(PBGO) $(GOSRCS)
	go mod tidy
	go build -ldflags="$(LDFLAGS)" -o $@

docker/build:
	docker build \
		--build-arg GIT_HASH=$(GIT_HASH) \
		--build-arg GIT_TAG=$(GIT_TAG) \
		-t $(DOCKER_IMAGE) \
		-f .docker/Dockerfile \
		.

docker/run:
	docker run \
		--env GEOIP2_LICENSE_KEY=$(GEOIP2_LICENSE_KEY) \
		-p 8080:8080 \
		-it --rm \
		-t $(DOCKER_IMAGE)

clean/proto:
	rm -f $(PBGO)

clean: clean/golangci-lint clean/protoc clean/protoc-gen-go clean/proto clean/watcher
	rm -f go.sum
	rm -f $(EXEC)

.PHONY: all tidy lint clean test
