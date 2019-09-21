import logging
import socketserver
import resource
from werkzeug.serving import run_simple
from . import app

fh = logging.FileHandler('/var/log/net-dhcp.log')
fh.setFormatter(logging.Formatter('%(asctime)s [%(levelname)s] %(message)s'))

logger = logging.getLogger('net-dhcp')
logger.setLevel(logging.DEBUG)
logger.addHandler(fh)

resource.setrlimit(resource.RLIMIT_NOFILE, (1048576, 1048576))

socketserver.TCPServer.allow_reuse_address = True
run_simple('unix:///run/docker/plugins/net-dhcp.sock', 0, app)
