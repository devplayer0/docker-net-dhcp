FROM python:3-alpine

COPY requirements.txt /opt/
RUN pip install -r /opt/requirements.txt

RUN mkdir -p /opt/plugin /run/docker/plugins
COPY plugin.sh /opt/plugin/launch.sh
COPY net-dhcp/ /opt/plugin/net_dhcp
WORKDIR /opt/plugin
ENV GUNICORN_OPTS="--log-level=DEBUG"
ENTRYPOINT [ "/opt/plugin/launch.sh" ]
