#!/bin/sh
exec gunicorn $GUNICORN_OPTS --workers 1 --bind unix:/run/docker/plugins/net-dhcp.sock net_dhcp:app
