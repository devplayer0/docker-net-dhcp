#!/usr/bin/env python
import sys
from os import environ as env

EVENT_PREFIX = '__event'

if __name__ != '__main__':
    print('You shouldn\'t be importing this script!')
    sys.exit(1)

event_type = sys.argv[1]
if event_type in ('bound', 'renew'):
    print(f'{EVENT_PREFIX} {event_type} {env["ip"]}/{env["mask"]} {env["router"]} {env["domain"]}')
elif event_type in ('deconfig', 'leasefail', 'nak'):
    print(f'{EVENT_PREFIX} {event_type}')
else:
    print(f'unknown udhcpc event "{event_type}"')
    sys.exit(1)
