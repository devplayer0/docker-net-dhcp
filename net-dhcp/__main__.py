import logging
import socketserver
from werkzeug.serving import run_simple
from . import app

fh = logging.FileHandler('/var/log/net-dhcp.log')
fh.setFormatter(logging.Formatter('%(asctime)s [%(levelname)s] %(message)s'))

logger = logging.getLogger('net-dhcp')
logger.setLevel(logging.DEBUG)
logger.addHandler(fh)

socketserver.TCPServer.allow_reuse_address = True
run_simple('unix:///run/docker/plugins/net-dhcp.sock', 0, app)
