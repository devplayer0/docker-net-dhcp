# docker-net-dhcp
`docker-net-dhcp` is a Docker plugin providing a network driver which allocates IP addresses (IPv4 and optionally IPv6)
via an existing DHCP server (e.g. your router).

When configured correctly, this allows you to spin up a container (e.g. `docker run ...` or `docker-compose up ...`) and
access it on your network as if it was any other machine!

# Usage
## Installation
```
$ docker plugin install devplayer0/net-dhcp
Plugin "devplayer0/net-dhcp" is requesting the following privileges:
 - network: [host]
 - host pid namespace: [true]
 - mount: [/var/run/docker.sock]
 - capabilities: [CAP_NET_ADMIN CAP_SYS_ADMIN]
Do you grant the above permissions? [y/N] y
latest: Pulling from devplayer0/net-dhcp
<some id>: Download complete 
Digest: sha256:<some hash>
Status: Downloaded newer image for devplayer0/net-dhcp:latest
Installed plugin devplayer0/net-dhcp
$
```

## Network creation
In order to create a Docker network using `net-dhcp`, you'll need a pre-configured bridge interface on the host. How you
set this up will depend on your system, but the following (manual) instructions should work on most Linux distros:
```
# Create the bridge
$ sudo ip link add my-bridge type bridge
$ sudo ip link set my-bridge up

# Assuming 'eth0' is connected to your LAN (where the DHCP server is)
$ sudo ip link set eth0 up
# Attach your network card to the bridge
$ sudo ip link set eth0 master my-bridge

# Get an IP for the host (will go out to the DHCP server since eth0 is attached to the bridge)
$ sudo dhcpcd my-bridge
```

Once the bridge is ready, you can create the network:
```
$ docker network create -d devplayer0/net-dhcp:latest --ipam-driver null -o bridge=my-bridge my-dhcp-net
<some network id>
$

# With IPv6 enabled
# Although `docker network create` has a `--ipv6` flag, it doesn't work with the null IPAM driver
$ docker network create -d devplayer0/net-dhcp:latest --ipam-driver null -o bridge=test -o ipv6=true my-dhcp-net
<some network id>
$
```
_Note: The `null` IPAM driver **must** be used, or else Docker will try to allocate IP addresses from its choice of
subnet - this can cause IP conflicts since the bridge is connected to your local network!_

## Container creation
Once you've created a network, you can create some containers:
```
$ docker run --rm -ti --network my-dhcp-net alpine
/ # ip address show
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
159: my-bridge0@if160: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1500 qdisc noqueue state UP qlen 1000
    link/ether 86:41:68:f8:85:b9 brd ff:ff:ff:ff:ff:ff
    inet 10.255.0.246/24 brd 10.255.0.255 scope global test0
       valid_lft forever preferred_lft forever
/ # ip route show
default via 10.255.0.123 dev my-bridge0 
10.255.0.0/24 dev my-bridge0 scope link  src 10.255.0.246 
/ #
```
Note:
 - It will take a bit longer than usual for the container to start, as a DHCP lease needs to be obtained before creating it
 - Once created, a persistent DHCP client will renew the DHCP lease (and then update the default gateway in the
 container) when necessary - **this client runs separately from the container**
