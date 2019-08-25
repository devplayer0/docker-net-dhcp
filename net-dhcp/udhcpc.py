from enum import Enum
import ipaddress
from os import path
import threading
import subprocess
import logging

from pyroute2.netns.process.proxy import NSPopen

INFO_PREFIX = '__info'
HANDLER_SCRIPT = path.join(path.dirname(__file__), 'udhcpc_handler.py')

class EventType(Enum):
    BOUND = 'bound'
    RENEW = 'renew'

logger = logging.getLogger('gunicorn.error')

class DHCPClientError(Exception):
    pass

def _nspopen_wrapper(netns):
    return lambda *args, **kwargs: NSPopen(netns, *args, **kwargs)
class DHCPClient:
    def __init__(self, iface, netns=None, once=False, event_listener=lambda t, ip, gw, dom: None):
        self.netns = netns
        self.iface = iface
        self.once = once
        self.event_listener = event_listener

        Popen = _nspopen_wrapper(netns) if netns else subprocess.Popen
        cmdline = ['/sbin/udhcpc', '-s', HANDLER_SCRIPT, '-i', iface, '-f']
        cmdline.append('-q' if once else '-R')
        self.proc = Popen(cmdline, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, encoding='utf-8')

        self.ip = None
        self.gateway = None
        self.domain = None
        self._event_thread = threading.Thread(target=self._read_events)
        self._event_thread.start()

    def _read_events(self):
        while True:
            line = self.proc.stdout.readline()
            if not line:
                break
            if not line.startswith(INFO_PREFIX):
                logger.debug('[udhcpc] %s', line)
                continue

            args = line.split(' ')[1:]
            try:
                event_type = EventType(args[0])
            except ValueError:
                logger.warning('udhcpc unknown event "%s"', ' '.join(args))
                continue

            self.ip = ipaddress.ip_interface(args[1])
            self.gateway = ipaddress.ip_address(args[2])
            self.domain = args[3]

            logger.debug('[udhcp event] %s %s %s %s', event_type, self.ip, self.gateway, self.domain)
            self.event_listener(event_type, self.ip, self.gateway, self.domain)

    def finish(self):
        if not self.once:
            self.proc.terminate()
        if self.proc.wait(timeout=10) != 0:
            raise DHCPClientError(f'udhcpc exited with non-zero exit code {self.proc.returncode}')
        self._event_thread.join()

        if self.once and not self.ip:
            raise DHCPClientError(f'failed to obtain lease')
