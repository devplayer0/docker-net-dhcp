PLUGIN_NAME = ghcr.io/devplayer0/docker-net-dhcp
PLUGIN_TAG ?= golang
PLATFORMS ?= linux/amd64,linux/arm64/v8

SOURCES = $(shell find pkg/ cmd/ -name '*.go')
BINARY = bin/net-dhcp

.PHONY: all debug build create enable disable pdebug push clean

all: create enable

bin/%: $(SOURCES)
	go build -o $@ ./cmd/$(shell basename $@)

debug: $(BINARY)
	sudo $< -log debug

build: $(SOURCES)
	docker build -t $(PLUGIN_NAME):rootfs .

plugin/rootfs: build
	mkdir -p plugin/rootfs
	docker create --name tmp $(PLUGIN_NAME):rootfs
	docker export tmp | tar xC plugin/rootfs
	docker rm -vf tmp

plugin: plugin/rootfs config.json
	cp config.json $@/

create: plugin
	docker plugin rm -f $(PLUGIN_NAME):$(PLUGIN_TAG) || true
	docker plugin create $(PLUGIN_NAME):$(PLUGIN_TAG) $<
	docker plugin set $(PLUGIN_NAME):$(PLUGIN_TAG) LOG_LEVEL=trace

enable: plugin
	docker plugin enable $(PLUGIN_NAME):$(PLUGIN_TAG)
disable:
	docker plugin disable $(PLUGIN_NAME):$(PLUGIN_TAG)

pdebug: create enable
	sudo sh -c 'tail -f /var/lib/docker/plugins/*/rootfs/var/log/net-dhcp.log'

push: create
	docker plugin push $(PLUGIN_NAME):$(PLUGIN_TAG)

multiarch: $(SOURCES)
	docker buildx build --platform=$(PLATFORMS) -o type=local,dest=$@ .

push-multiarch: multiarch config.json
	scripts/push_multiarch_plugin.py -p $(PLATFORMS) config.json multiarch $(PLUGIN_NAME):$(PLUGIN_TAG)

clean:
	-rm -rf multiarch/
	-rm -rf plugin/
	-rm bin/*
