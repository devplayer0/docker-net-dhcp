from flask import Flask, jsonify

app = Flask(__name__)

from . import network, ipam

@app.errorhandler(404)
def err_not_found(e):
    return jsonify({'Err': 'API not found'}), 404

@app.errorhandler(Exception)
def err(e):
    return jsonify({'Err': f'Error: {e}'}), 500
