PLUGIN_NAME = devplayer0/net-dhcp
PLUGIN_TAG ?= latest

all: clean rootfs build create

clean:
	@echo "### rm ./plugin"
	@rm -rf ./plugin

build:
	@echo "### docker build: rootfs image with net-dhcp"
	@docker build -t ${PLUGIN_NAME}:rootfs .

rootfs:
	@echo "### create rootfs directory in ./plugin/rootfs"
	@mkdir -p ./plugin/rootfs
	@docker create --name tmp ${PLUGIN_NAME}:rootfs
	@docker export tmp | tar -x -C ./plugin/rootfs
	@echo "### copy config.json to ./plugin/"
	@cp config.json ./plugin/
	@docker rm -vf tmp

create:
	@echo "### remove existing plugin ${PLUGIN_NAME}:${PLUGIN_TAG} if exists"
	@docker plugin rm -f ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	@echo "### create new plugin ${PLUGIN_NAME}:${PLUGIN_TAG} from ./plugin"
	@docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} ./plugin

debug:
	@docker run --rm -ti --cap-add CAP_SYS_ADMIN --network host --volume /run/docker/plugins:/run/docker/plugins \
		--volume /run/docker.sock:/run/docker.sock --volume /var/run/docker/netns:/var/run/docker/netns \
		${PLUGIN_NAME}:rootfs

enable:
	@echo "### enable plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"		
	@docker plugin enable ${PLUGIN_NAME}:${PLUGIN_TAG}

push: clean rootfs create enable
	@echo "### push plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker plugin push ${PLUGIN_NAME}:${PLUGIN_TAG}
