TOOLCHAIN ?= .tool
PROTOC := $(TOOLCHAIN)/bin/protoc

$(PROTOC):
	mkdir -p $(dir $@)
ifeq ($(shell uname -s),Linux)
	curl -L -o $(TOOLCHAIN)/protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v3.11.4/protoc-3.11.4-linux-x86_64.zip
else ifeq ($(shell uname -s),Darwin)
	curl -L -o $(TOOLCHAIN)/protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v3.11.4/protoc-3.11.4-osx-x86_64.zip
endif
	cd $(TOOLCHAIN) && unzip -o protoc.zip
	chmod +x $@
	rm -f $(TOOLCHAIN)/protoc.zip

clean/protoc:
	rm -f $(PROTOC)

.PHONY: clean/protoc
