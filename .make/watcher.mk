TOOLCHAIN ?= .tool
WATCHER := $(TOOLCHAIN)/bin/watcher

$(WATCHER): 
	mkdir -p $(dir $(TOOLCHAIN))
	GOPATH=$(shell pwd)/$(TOOLCHAIN) go get github.com/crosstalkio/go-watcher
	GOPATH=$(shell pwd)/$(TOOLCHAIN) go install github.com/crosstalkio/go-watcher/cmd/watcher

clean/watcher:
	rm -rf $(WATCHER)

.PHONY: clean/watcher
