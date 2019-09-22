import itertools
import ipaddress
import logging
import atexit
import socket
import time
import threading
import subprocess

import pyroute2
from pyroute2.netlink.rtnl import rtypes
import docker
from flask import request, jsonify

from . import NetDhcpError, udhcpc, app

OPTS_KEY = 'com.docker.network.generic'
OPT_BRIDGE = 'bridge'
OPT_IPV6 = 'ipv6'

logger = logging.getLogger('net-dhcp')

ndb = pyroute2.NDB()
@atexit.register
def close_ndb():
    ndb.close()

client = docker.from_env()
@atexit.register
def close_docker():
    client.close()

gateway_hints = {}
container_dhcp_clients = {}
@atexit.register
def cleanup_dhcp():
    for endpoint, dhcp in container_dhcp_clients.items():
        logger.warning('cleaning up orphaned container DHCP client (endpoint "%s")', endpoint)
        dhcp.stop()

def veth_pair(e):
    return f'dh-{e[:12]}', f'{e[:12]}-dh'

def iface_addrs(iface):
    return list(map(lambda a: ipaddress.ip_interface((a['address'], a['prefixlen'])), iface.ipaddr))
def iface_nets(iface):
    return list(map(lambda n: n.network, iface_addrs(iface)))

def get_bridges():
    reserved_nets = set(map(ipaddress.ip_network, map(lambda c: c['Subnet'], \
        itertools.chain.from_iterable(map(lambda i: i['Config'], filter(lambda i: i['Driver'] != 'net-dhcp', \
            map(lambda n: n.attrs['IPAM'], client.networks.list())))))))

    return dict(map(lambda i: (i['ifname'], i), filter(lambda i: i['kind'] == 'bridge' and not \
        set(iface_nets(i)).intersection(reserved_nets), map(lambda i: ndb.interfaces[i.ifname], ndb.interfaces))))

def net_bridge(n):
    return ndb.interfaces[client.networks.get(n).attrs['Options'][OPT_BRIDGE]]
def ipv6_enabled(n):
    options = client.networks.get(n).attrs['Options']
    return OPT_IPV6 in options and options[OPT_IPV6] == 'true'

def endpoint_container_iface(n, e):
    for cid, info in client.networks.get(n).attrs['Containers'].items():
        if info['EndpointID'] == e:
            container = client.containers.get(cid)
            netns = f'/proc/{container.attrs["State"]["Pid"]}/ns/net'

            with pyroute2.NetNS(netns) as rtnl:
                for link in rtnl.get_links():
                    attrs = dict(link['attrs'])
                    if attrs['IFLA_ADDRESS'] == info['MacAddress']:
                        return {
                            'netns': netns,
                            'ifname': attrs['IFLA_IFNAME'],
                            'address': attrs['IFLA_ADDRESS']
                        }
            break
    return None
def await_endpoint_container_iface(n, e, timeout=5):
    start = time.time()
    iface = None
    while time.time() - start < timeout:
        try:
            iface = endpoint_container_iface(n, e)
        except docker.errors.NotFound:
            time.sleep(0.5)
    if not iface:
        raise NetDhcpError('Timed out waiting for container to become availabile')
    return iface

def endpoint_container_hostname(n, e):
    for cid, info in client.networks.get(n).attrs['Containers'].items():
        if info['EndpointID'] == e:
            return client.containers.get(cid).attrs['Config']['Hostname']
    return None

@app.route('/NetworkDriver.GetCapabilities', methods=['POST'])
def net_get_capabilities():
    return jsonify({
        'Scope': 'local',
        'ConnectivityScope': 'global'
    })

@app.route('/NetworkDriver.CreateNetwork', methods=['POST'])
def create_net():
    req = request.get_json(force=True)
    for data in req['IPv4Data']:
        if data['AddressSpace'] != 'null' or data['Pool'] != '0.0.0.0/0':
            return jsonify({'Err': 'Only the null IPAM driver is supported'}), 400

    options = req['Options'][OPTS_KEY]
    if OPT_BRIDGE not in options:
        return jsonify({'Err': 'No bridge provided'}), 400
    # We have to use a custom "enable IPv6" option because Docker's null IPAM driver doesn't support IPv6 and a plugin
    # IPAM driver isn't allowed to return an empty address
    if OPT_IPV6 in options and options[OPT_IPV6] not in ('', 'true', 'false'):
        return jsonify({'Err': 'Invalid boolean value for ipv6'}), 400

    desired = options[OPT_BRIDGE]
    bridges = get_bridges()
    if desired not in bridges:
        return jsonify({'Err': f'Bridge "{desired}" not found (or the specified bridge is already used by Docker)'}), 400

    logger.info('Creating network "%s" (using bridge "%s")', req['NetworkID'], desired)
    return jsonify({})

@app.route('/NetworkDriver.DeleteNetwork', methods=['POST'])
def delete_net():
    return jsonify({})

@app.route('/NetworkDriver.CreateEndpoint', methods=['POST'])
def create_endpoint():
    req = request.get_json(force=True)
    network_id = req['NetworkID']
    endpoint_id = req['EndpointID']
    req_iface = req['Interface']

    bridge = net_bridge(network_id)
    bridge_addrs = iface_addrs(bridge)

    if_host, if_container = veth_pair(endpoint_id)
    logger.info('creating veth pair %s <=> %s', if_host, if_container)
    if_host = (ndb.interfaces.create(ifname=if_host, kind='veth', peer=if_container)
                .set('state', 'up')
                .commit())

    try:
        start = time.time()
        while isinstance(if_container, str) and time.time() - start < 10:
            try:
                if_container = (ndb.interfaces[if_container]
                                .set('state', 'up')
                                .commit())
            except KeyError:
                time.sleep(0.5)
        if isinstance(if_container, str):
            raise NetDhcpError(f'timed out waiting for {if_container} to appear in host')

        (bridge
            .add_port(if_host)
            .commit())

        res_iface = {
            'MacAddress': '',
            'Address': '',
            'AddressIPv6': ''
        }

        if 'MacAddress' in req_iface and req_iface['MacAddress']:
            (if_container
            .set('address', req_iface['MacAddress'])
            .commit())
        else:
            res_iface['MacAddress'] = if_container['address']

        def try_addr(type_):
            addr = None
            k = 'AddressIPv6' if type_ == 'v6' else 'Address'
            if k in req_iface and req_iface[k]:
                # TODO: Should we allow static IP's somehow?
                # Just validate the address, Docker will add it to the interface for us
                #addr = ipaddress.ip_interface(req_iface[k])
                #for bridge_addr in bridge_addrs:
                #    if addr.ip == bridge_addr.ip:
                #        raise NetDhcpError(400, f'Address {addr} is already in use on bridge {bridge["ifname"]}')
                raise NetDhcpError('Only the null IPAM driver is supported')
            else:
                dhcp = udhcpc.DHCPClient(if_container, v6=type_ == 'v6', once=True)
                addr = dhcp.finish()
                if not addr:
                    return
                res_iface[k] = str(addr)

                if dhcp.gateway:
                    gateway_hints[endpoint_id] = dhcp.gateway
            logger.info('Adding IP%s address %s to %s', type_, addr, if_container['ifname'])

        try_addr('v4')
        if ipv6_enabled(network_id):
            try_addr('v6')

        res = jsonify({
            'Interface': res_iface
        })
    except Exception as e:
        logger.exception(e)

        if not isinstance(if_container, str):
            (bridge
                .del_port(if_host)
                .commit())
        (if_host
            .remove()
            .commit())

        if isinstance(e, NetDhcpError):
            res = jsonify({'Err': str(e)}), e.status
        else:
            res = jsonify({'Err': str(e)}), 500
    finally:
        return res

@app.route('/NetworkDriver.EndpointOperInfo', methods=['POST'])
def endpoint_info():
    req = request.get_json(force=True)

    bridge = net_bridge(req['NetworkID'])
    if_host, _if_container = veth_pair(req['EndpointID'])
    if_host = ndb.interfaces[if_host]

    return jsonify({
        'bridge': bridge['ifname'],
        'if_host': {
            'name': if_host['ifname'],
            'mac': if_host['address']
        }
    })

@app.route('/NetworkDriver.DeleteEndpoint', methods=['POST'])
def delete_endpoint():
    req = request.get_json(force=True)

    bridge = net_bridge(req['NetworkID'])
    if_host, _if_container = veth_pair(req['EndpointID'])
    if_host = ndb.interfaces[if_host]

    bridge.del_port(if_host['ifname'])
    (if_host
        .remove()
        .commit())

    return jsonify({})

@app.route('/NetworkDriver.Join', methods=['POST'])
def join():
    req = request.get_json(force=True)
    network = req['NetworkID']
    endpoint = req['EndpointID']

    bridge = net_bridge(req['NetworkID'])
    _if_host, if_container = veth_pair(req['EndpointID'])

    res = {
        'InterfaceName': {
            'SrcName': if_container,
            'DstPrefix': bridge['ifname']
        },
        'StaticRoutes': []
    }

    if endpoint in gateway_hints:
        gateway = gateway_hints[endpoint]
        logger.info('Setting IPv4 gateway from DHCP (%s)', gateway)
        res['Gateway'] = str(gateway)
        del gateway_hints[endpoint]

    ipv6 = ipv6_enabled(network)
    for route in bridge.routes:
        if route['type'] != rtypes['RTN_UNICAST'] or \
            (route['family'] == socket.AF_INET6 and not ipv6):
            continue

        if route['dst'] in ('', '/0'):
            if route['family'] == socket.AF_INET and 'Gateway' not in res:
                logger.info('Adding IPv4 gateway %s', route['gateway'])
                res['Gateway'] = route['gateway']
            elif route['family'] == socket.AF_INET6 and 'GatewayIPv6' not in res:
                logger.info('Adding IPv6 gateway %s', route['gateway'])
                res['GatewayIPv6'] = route['gateway']
        elif route['gateway']:
            dst = f'{route["dst"]}/{route["dst_len"]}'
            logger.info('Adding route to %s via %s', dst, route['gateway'])
            res['StaticRoutes'].append({
                'Destination': dst,
                'RouteType': 0,
                'NextHop': route['gateway']
            })

    container_dhcp_clients[endpoint] = ContainerDHCPManager(network, endpoint)
    return jsonify(res)

@app.route('/NetworkDriver.Leave', methods=['POST'])
def leave():
    req = request.get_json(force=True)
    endpoint = req['EndpointID']

    if endpoint in container_dhcp_clients:
        container_dhcp_clients[endpoint].stop()
        del container_dhcp_clients[endpoint]

    return jsonify({})

# Trying to grab the container's attributes (to get the network namespace)
# will deadlock (since Docker is waiting on us), so we must defer starting
# the DHCP client
class ContainerDHCPManager:
    def __init__(self, network, endpoint):
        self.network = network
        self.endpoint = endpoint
        self.ipv6 = ipv6_enabled(network)

        self.dhcp = None
        self.dhcp6 = None
        self._thread = threading.Thread(target=self.run)
        self._thread.start()

    def _on_event(self, dhcp, event_type, _event):
        if event_type != udhcpc.EventType.RENEW or not dhcp.gateway:
            return

        logger.info('[dhcp container] Replacing gateway with %s', dhcp.gateway)
        subprocess.check_call(['nsenter', f'-n{dhcp.netns}', '--', '/sbin/ip', 'route', 'replace', 'default', 'via',
            str(dhcp.gateway)], timeout=1, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL)

        # TODO: Adding default route with NDB seems to be broken (because of the dst syntax?)
        #for route in ndb.routes:
        #    if route['type'] != rtypes['RTN_UNICAST'] or \
        #        route['oif'] != dhcp.iface['index'] or \
        #        (route['family'] == socket.AF_INET6 and not self.ipv6) or \
        #        route['dst'] not in ('', '/0'):
        #        continue

        #    # Needed because Route.remove() doesn't like a blank destination
        #    logger.info('Removing default route via %s', route['gateway'])
        #    route['dst'] = '::' if route['family'] == socket.AF_INET6 else '0.0.0.0'
        #    (route
        #        .remove()
        #        .commit())

        #logger.info('Adding default route via %s', dhcp.gateway)
        #(ndb.routes.add({'oif': dhcp.iface['index'], 'gateway': dhcp.gateway})
        #    .commit())

    def run(self):
        try:
            iface = await_endpoint_container_iface(self.network, self.endpoint)
            hostname = endpoint_container_hostname(self.network, self.endpoint)

            self.dhcp = udhcpc.DHCPClient(iface, event_listener=self._on_event, hostname=hostname)
            logger.info('Starting DHCPv4 client on %s in container namespace %s', iface['ifname'], \
                self.dhcp.netns)

            if self.ipv6:
                self.dhcp6 = udhcpc.DHCPClient(iface, v6=True, event_listener=self._on_event, hostname=hostname)
                logger.info('Starting DHCPv6 client on %s in container namespace %s', iface['ifname'], \
                    self.dhcp6.netns)
        except Exception as e:
            logger.exception(e)
            if self.dhcp:
                self.dhcp.finish(timeout=1)

    def stop(self):
        if not self.dhcp:
            return

        try:
            logger.info('Shutting down DHCPv4 client on %s in container namespace %s', \
                self.dhcp.iface['ifname'], self.dhcp.netns)
            self.dhcp.finish(timeout=1)
        finally:
            try:
                if self.ipv6:
                    logger.info('Shutting down DHCPv6 client on %s in container namespace %s', \
                        self.dhcp6.iface['ifname'], self.dhcp.netns)
                    self.dhcp6.finish(timeout=1)
            finally:
                self._thread.join()

                # we have to do this since the docker client leaks sockets...
                global client
                client.close()
                client = docker.from_env()
