from flask import jsonify

from . import app

@app.route('/IpamDriver.GetCapabilities')
def ipam_get_capabilities():
    return jsonify({
        'RequiresMACAddress': True,
        'RequiresRequestReplay': False
    })