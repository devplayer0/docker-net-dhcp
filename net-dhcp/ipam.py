import itertools
import ipaddress
from os import path
import atexit

import pyroute2
import docker
from flask import jsonify

from . import app

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

    return list(filter(lambda i: i.kind == 'bridge' and not set(iface_nets(i)).intersection(reserved_nets), \
        map(lambda i: ipdb.interfaces[i], filter(lambda k: isinstance(k, str), ipdb.interfaces.keys()))))

@app.route('/IpamDriver.GetCapabilities')
def ipam_get_capabilities():
    return jsonify({
        'RequiresMACAddress': True,
        'RequiresRequestReplay': False
    })

@app.route('/IpamDriver.GetDefaultAddressSpace')
def get_default_addr_space():
    bridges = get_bridges()
    if not bridges:
        return jsonify({'Err': 'No bridges available'}), 404

    first = None
    for b in bridges:
        if b.ipaddr.ipv4:
            first = b
    if not first:
        return jsonify({'Err': 'No bridges with addresses available'}), 404

    return jsonify({
        'LocalDefaultAddressSpace': f'{first.ifname}-{iface_addrs(first)[0].network}',
        'GlobalDefaultAddressSpace': None
    })