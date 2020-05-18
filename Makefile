PLUGIN_NAME = devplayer0/net-dhcp
PLUGIN_TAG ?= golang

BINARY = bin/net-dhcp
PLUGIN_DIR = plugin

.PHONY: all clean disable

all: create enable

$(BINARY): cmd/net-dhcp/main.go
	CGO_ENABLED=0 go build -o $@ ./cmd/net-dhcp

debug: $(BINARY)
	sudo $< -log debug

plugin: $(BINARY) config.json
	mkdir -p $@/rootfs/run/docker/plugins
	cp $(BINARY) $@/rootfs/
	cp config.json $@/

create: plugin
	docker plugin rm -f ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} $<

enable: plugin
	docker plugin enable ${PLUGIN_NAME}:${PLUGIN_TAG}
disable:
	docker plugin disable ${PLUGIN_NAME}:${PLUGIN_TAG}

pdebug: create enable
	sudo sh -c 'tail -f /var/lib/docker/plugins/*/rootfs/net-dhcp.log'

push: plugin
	docker plugin push ${PLUGIN_NAME}:${PLUGIN_TAG}

clean:
	-rm -rf ./plugin
	-rm - bin/*
