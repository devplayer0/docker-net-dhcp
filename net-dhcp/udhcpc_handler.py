#!/usr/bin/env python
import json
import sys
from os import environ as env

import posix_ipc

if __name__ != '__main__':
    print('You shouldn\'t be importing this script!')
    sys.exit(1)

event = {'type': sys.argv[1]}
if event['type'] in ('bound', 'renew'):
    event['ip'] = f'{env["ip"]}/{env["mask"]}'
    if 'router' in env:
        event['router'] = env['router']
    if 'domain' in env:
        event['domain'] = env['domain']
elif event['type'] in ('deconfig', 'leasefail', 'nak'):
    pass
else:
    event['type'] = 'unknown'

queue = posix_ipc.MessageQueue(env['EVENT_QUEUE'])
queue.send(json.dumps(event))
queue.close()
