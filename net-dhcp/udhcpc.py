from enum import Enum
import ipaddress
import json
import struct
import binascii
import os
from os import path
from select import select
import threading
import subprocess
import logging

from eventfd import EventFD
import posix_ipc
from pyroute2.netns.process.proxy import NSPopen

HANDLER_SCRIPT = path.join(path.dirname(__file__), 'udhcpc_handler.py')
AWAIT_INTERVAL = 0.1
VENDOR_ID = 'docker'

class EventType(Enum):
    BOUND = 'bound'
    RENEW = 'renew'
    DECONFIG = 'deconfig'
    LEASEFAIL = 'leasefail'

logger = logging.getLogger('gunicorn.error')

class DHCPClientError(Exception):
    pass

def _nspopen_wrapper(netns):
    return lambda *args, **kwargs: NSPopen(netns, *args, **kwargs)
class DHCPClient:
    def __init__(self, iface, v6=False, once=False, hostname=None, event_listener=None):
        self.iface = iface
        self.v6 = v6
        self.once = once
        self.event_listeners = [DHCPClient._attr_listener]
        if event_listener:
            self.event_listeners.append(event_listener)

        self.netns = None
        if iface['target'] and iface['target'] != 'localhost':
            self.netns = iface['target']
            logger.debug('udhcpc using netns %s', self.netns)

        Popen = _nspopen_wrapper(self.netns) if self.netns else subprocess.Popen
        bin_path = '/usr/bin/udhcpc6' if v6 else '/sbin/udhcpc'
        cmdline = [bin_path, '-s', HANDLER_SCRIPT, '-i', iface['ifname'], '-f']
        cmdline.append('-q' if once else '-R')
        if hostname:
            cmdline.append('-x')
            if v6:
                # TODO: We encode the fqdn for DHCPv6 because udhcpc6 seems to be broken
                # flags: S bit set (see RFC4704)
                enc_hostname = hostname.encode('utf-8')
                enc_hostname = struct.pack('BB', 0b0001, len(enc_hostname)) + enc_hostname
                enc_hostname = binascii.hexlify(enc_hostname).decode('ascii')
                hostname_opt = f'0x27:{enc_hostname}'
            else:
                hostname_opt = f'hostname:{hostname}'
            cmdline.append(hostname_opt)
        if not v6:
            cmdline += ['-V', VENDOR_ID]

        self._suffix = '6' if v6 else ''
        self._event_queue = posix_ipc.MessageQueue(f'/udhcpc{self._suffix}_{iface["address"].replace(":", "_")}', \
            flags=os.O_CREAT | os.O_EXCL)
        self.proc = Popen(cmdline, env={'EVENT_QUEUE': self._event_queue.name})
        if hostname:
            logger.debug('[udhcpc%s#%d] using hostname "%s"', self._suffix, self.proc.pid, hostname)

        self._has_lease = threading.Event()
        self.ip = None
        self.gateway = None
        self.domain = None

        self._shutdown_event = EventFD()
        self._event_thread = threading.Thread(target=self._read_events)
        self._event_thread.start()

    def _attr_listener(self, event_type, event):
        if event_type in (EventType.BOUND, EventType.RENEW):
            self.ip = ipaddress.ip_interface(event['ip'])
            if 'gateway' in event:
                self.gateway = ipaddress.ip_address(event['gateway'])
            else:
                self.gateway = None
            self.domain = event.get('domain')
            self._has_lease.set()
        elif event_type == EventType.DECONFIG:
            self._has_lease.clear()
            self.ip = None
            self.gateway = None
            self.domain = None

    def _read_events(self):
        while True:
            r, _w, _e = select([self._shutdown_event, self._event_queue.mqd], [], [])
            if self._shutdown_event in r:
                break

            msg, _priority = self._event_queue.receive()
            event = json.loads(msg.decode('utf-8'))
            try:
                event['type'] = EventType(event['type'])
            except ValueError:
                logger.warning('udhcpc%s#%d unknown event "%s"', self._suffix, self.proc.pid, event)
                continue

            logger.debug('[udhcp%s#%d event] %s', self._suffix, self.proc.pid, event)
            for listener in self.event_listeners:
                try:
                    listener(self, event['type'], event)
                except Exception as ex:
                    logger.exception(ex)

    def await_ip(self, timeout=10):
        if not self._has_lease.wait(timeout=timeout):
            raise DHCPClientError(f'Timed out waiting for lease from udhcpc{self._suffix}')

        return self.ip

    def finish(self, timeout=5):
        if self._shutdown_event.is_set():
            return

        try:
            if self.proc.returncode is not None and (not self.once or self.proc.returncode != 0):
                raise DHCPClientError(f'udhcpc{self._suffix} exited early with code {self.proc.returncode}')
            if self.once:
                self.await_ip()
            else:
                self.proc.terminate()

            if self.proc.wait(timeout=timeout) != 0:
                raise DHCPClientError(f'udhcpc{self._suffix} exited with non-zero exit code {self.proc.returncode}')

            return self.ip
        finally:
            if self.netns:
                self.proc.release()

            self._shutdown_event.set()
            self._event_thread.join()
            self._event_queue.close()
            self._event_queue.unlink()
