#!/bin/sh
BRIDGE=net-dhcp
BRIDGE_IP="10.123.0.1/24"
DHCP_RANGE="10.123.0.5,10.123.0.254"
DOMAIN=cool-dhcp

quit() {
    ip link del "$BRIDGE"
    exit
}

trap quit SIGINT SIGTERM

ip link add "$BRIDGE" type bridge
ip link set up dev "$BRIDGE"
ip addr add "$BRIDGE_IP" dev "$BRIDGE"

dnsmasq --no-daemon --conf-file=/dev/null \
    --port=0 --interface="$BRIDGE" --bind-interfaces \
    --domain="$DOMAIN" \
    --dhcp-range="$DHCP_RANGE" --dhcp-leasefile=/dev/null

quit
