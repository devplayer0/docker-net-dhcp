import itertools
import ipaddress
from os import path
import logging
import atexit

import pyroute2
import docker
from flask import request, jsonify

from . import app

OPTS_KEY = 'com.docker.network.generic' 
BRIDGE_OPT = 'devplayer0.net-dhcp.bridge'

logger = logging.getLogger('gunicorn.error')

ipdb = pyroute2.IPDB()
@atexit.register
def close_ipdb():
    ipdb.release()

client = docker.from_env()
@atexit.register
def close_docker():
    client.close()

def iface_addrs(iface):
    return list(map(ipaddress.ip_interface, iface.ipaddr.ipv4))
def iface_nets(iface):
    return list(map(lambda n: n.network, map(ipaddress.ip_interface, iface.ipaddr.ipv4)))

def get_bridges():
    reserved_nets = set(map(ipaddress.ip_network, map(lambda c: c['Subnet'], \
        itertools.chain.from_iterable(map(lambda i: i['Config'], filter(lambda i: i['Driver'] != 'net-dhcp', \
            map(lambda n: n.attrs['IPAM'], client.networks.list())))))))

    return dict(map(lambda i: (i.ifname, i), filter(lambda i: i.kind == 'bridge' and not \
        set(iface_nets(i)).intersection(reserved_nets), map(lambda i: ipdb.interfaces[i], \
            filter(lambda k: isinstance(k, str), ipdb.interfaces.keys())))))

@app.route('/NetworkDriver.GetCapabilities', methods=['POST'])
def net_get_capabilities():
    return jsonify({
        'Scope': 'local',
        'ConnectivityScope': 'global'
    })

@app.route('/NetworkDriver.CreateNetwork', methods=['POST'])
def create_net():
    req = request.get_json(force=True)
    if BRIDGE_OPT not in req['Options'][OPTS_KEY]:
        return jsonify({'Err': 'No bridge provided'}), 400

    desired = req['Options'][OPTS_KEY][BRIDGE_OPT]
    bridges = get_bridges()
    if desired not in bridges:
        return jsonify({'Err': f'Bridge "{desired}" not found (or the specified bridge is already used by Docker)'}), 400

    if request.json['IPv6Data']:
        return jsonify({'Err': 'IPv6 is currently unsupported'}), 400

    logger.info(f'Creating network "{req["NetworkID"]}" (using bridge "{desired}")')
    return jsonify({})

@app.route('/NetworkDriver.DeleteNetwork', methods=['POST'])
def delete_net():
    return jsonify({})
