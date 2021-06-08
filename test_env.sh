#!/bin/sh
BRIDGE=net-dhcp
BRIDGE_IP="10.123.0.1"
DUMMY_IP="10.123.0.3"
MASK="24"
DHCP_RANGE="10.123.0.5,10.123.0.254,10s"
BRIDGE_IP6="fd69::1"
DUMMY_IP6="fd69::3"
MASK6="64"
DHCP6_RANGE="fd69::5,fd69::1000,64,10s"
DOMAIN=cool-dhcp

quit() {
    ip link del "$BRIDGE"
    exit
}

trap quit SIGINT SIGTERM

ip link add "$BRIDGE" type bridge
ip link set up dev "$BRIDGE"
ip addr add "$BRIDGE_IP/$MASK" dev "$BRIDGE"
ip addr add "$BRIDGE_IP6/$MASK6" dev "$BRIDGE"

ip route add 10.223.0.0/24 dev "$BRIDGE"
ip route add 10.224.0.0/24 via "$DUMMY_IP"
ip route add fd42::0/64 dev "$BRIDGE"
# TODO: This doesn't work right now because the route is added by Docker before
# router advertisement stuff is done :/
#ip route add fd43::0/64 via "$DUMMY_IP6"

dnsmasq --no-daemon --conf-file=/dev/null --dhcp-leasefile=/tmp/docker-net-dhcp.leases \
    --port=0 --interface="$BRIDGE" --bind-interfaces \
    --domain="$DOMAIN" \
    --dhcp-range="$DHCP_RANGE" \
    --dhcp-range="$DHCP6_RANGE" --enable-ra

quit
