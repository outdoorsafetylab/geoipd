CONFIG := local

VARS :=
VARS += GitTime=$(shell git show -s --format=%ct HEAD || echo "")
VARS += GitHash=$(shell git rev-parse --short HEAD)
VARS += GitTag=$(shell git describe --tags --exact-match 2>/dev/null || echo "")
LDFLAGS := $(addprefix -X main.,$(VARS))

backend: $(WATCHER)
	$(WATCHER) -c $(CONFIG)

clean/backend:

.PHONY: backend clean/backend
