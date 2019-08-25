#!/usr/bin/env python
import sys
from os import environ as env

import posix_ipc

if __name__ != '__main__':
    print('You shouldn\'t be importing this script!')
    sys.exit(1)

event_type = sys.argv[1]
if event_type in ('bound', 'renew'):
    event = f'{event_type} {env["ip"]}/{env["mask"]} {env["router"]} {env["domain"]}'
elif event_type in ('deconfig', 'leasefail', 'nak'):
    event = event_type
else:
    print(f'unknown udhcpc event "{event_type}"')
    sys.exit(1)

#with open(env['EVENT_FILE'], 'a') as event_file:
#    event_file.write(event + '\n')
queue = posix_ipc.MessageQueue(env['EVENT_QUEUE'])
queue.send(event)
queue.close()