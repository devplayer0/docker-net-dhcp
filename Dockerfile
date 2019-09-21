FROM python:3-alpine

COPY requirements.txt /opt/
RUN apk --no-cache add gcc musl-dev && \
    pip install -r /opt/requirements.txt && \
    apk --no-cache del gcc musl-dev

RUN mkdir -p /opt/plugin /run/docker/plugins /var/run/docker/netns
COPY net-dhcp/ /opt/plugin/net_dhcp

WORKDIR /opt/plugin
ENTRYPOINT ["python", "-m", "net_dhcp"]
