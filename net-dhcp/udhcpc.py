from enum import Enum
import ipaddress
import os
from os import path
import fcntl
import time
import threading
import subprocess
import logging

from pyroute2.netns.process.proxy import NSPopen

EVENT_PREFIX = '__event'
HANDLER_SCRIPT = path.join(path.dirname(__file__), 'udhcpc_handler.py')
AWAIT_INTERVAL = 0.1

class EventType(Enum):
    BOUND = 'bound'
    RENEW = 'renew'
    DECONFIG = 'deconfig'
    LEASEFAIL = 'leasefail'

logger = logging.getLogger('gunicorn.error')

class DHCPClientError(Exception):
    pass

def _nspopen_wrapper(netns):
    def _wrapper(*args, **kwargs):
        # We have to set O_NONBLOCK on stdout since NSPopen uses a global lock
        # on the object (e.g. deadlock if we try to readline() and terminate())
        proc = NSPopen(netns, *args, **kwargs)
        proc.stdout.fcntl(fcntl.F_SETFL, os.O_NONBLOCK)
        return proc
    return _wrapper
class DHCPClient:
    def __init__(self, iface, once=False, event_listener=None):
        self.iface = iface
        self.once = once
        self.event_listeners = [DHCPClient._attr_listener]
        if event_listener:
            self.event_listeners.append(event_listener)

        self.netns = None
        if iface['target'] and iface['target'] != 'localhost':
            self.netns = iface['target']
            logger.debug('udhcpc using netns %s', self.netns)

        Popen = _nspopen_wrapper(self.netns) if self.netns else subprocess.Popen
        cmdline = ['/sbin/udhcpc', '-s', HANDLER_SCRIPT, '-i', iface['ifname'], '-f']
        cmdline.append('-q' if once else '-R')
        self.proc = Popen(cmdline, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, encoding='utf-8')

        self._has_lease = threading.Event()
        self.ip = None
        self.gateway = None
        self.domain = None

        self._running = True
        self._event_thread = threading.Thread(target=self._read_events)
        self._event_thread.start()

    def _attr_listener(self, event_type, args):
        if event_type in (EventType.BOUND, EventType.RENEW):
            self.ip = ipaddress.ip_interface(args[0])
            self.gateway = ipaddress.ip_address(args[1])
            self.domain = args[2]
            self._has_lease.set()
        elif event_type == EventType.DECONFIG:
            self._has_lease.clear()
            self.ip = None
            self.gateway = None
            self.domain = None

    def _read_events(self):
        while self._running:
            line = self.proc.stdout.readline().strip()
            if not line:
                # stdout will be O_NONBLOCK if udhcpc is in a netns
                # We can't use select() since the file descriptor is from
                # the NSPopen proxy
                if self.netns and self._running:
                    time.sleep(0.1)
                continue

            if not line.startswith(EVENT_PREFIX):
                logger.debug('[udhcpc#%d] %s', self.proc.pid, line)
                continue

            args = line.split(' ')[1:]
            try:
                event_type = EventType(args[0])
            except ValueError:
                logger.warning('udhcpc#%d unknown event "%s"', self.proc.pid, args)
                continue

            logger.debug('[udhcp#%d event] %s %s', self.proc.pid, event_type, args[1:])
            for listener in self.event_listeners:
                listener(self, event_type, args[1:])

    def await_ip(self, timeout=5):
        if not self._has_lease.wait(timeout=timeout):
            raise DHCPClientError('Timed out waiting for dhcp lease')

        return self.ip

    def finish(self, timeout=5):
        if self.once:
            self.await_ip()
        else:
            self.proc.terminate()

        if self.proc.wait(timeout=timeout) != 0:
            raise DHCPClientError(f'udhcpc exited with non-zero exit code {self.proc.returncode}')
        if self.netns:
            self.proc.release()
        self._running = False
        self._event_thread.join()

        return self.ip
