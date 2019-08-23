import logging

from flask import Flask, jsonify

class NetDhcpError(Exception):
    def __init__(self, status, *args):
        Exception.__init__(self, *args)
        self.status = status

app = Flask(__name__)

from . import network

logger = logging.getLogger('gunicorn.error')

@app.errorhandler(404)
def err_not_found(_e):
    return jsonify({'Err': 'API not found'}), 404

@app.errorhandler(Exception)
def err(e):
    logger.exception(e)
    return jsonify({'Err': str(e)}), 500
