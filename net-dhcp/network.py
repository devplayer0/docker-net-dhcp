from flask import jsonify

from . import app

@app.route('/NetworkDriver.GetCapabilities')
def net_get_capabilities():
    return jsonify({
        'Scope': 'local',
        'ConnectivityScope': 'global'
    })