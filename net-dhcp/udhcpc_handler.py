#!/usr/bin/env python
import sys
from os import environ as env

INFO_PREFIX = '__info'

if __name__ != '__main__':
    print('You shouldn\'t be importing this script!')
    sys.exit(1)

event_type = sys.argv[1]
if event_type == 'bound' or event_type == 'renew':
    print(f'{INFO_PREFIX} {event_type} {env["ip"]}/{env["mask"]} {env["router"]} {env["domain"]}')
elif event_type == 'deconfig':
    print('udhcpc startup / lost lease')
elif event_type == 'leasefail':
    print('udhcpc failed to get a lease')
elif event_type == 'nak':
    print('udhcpc received NAK')
else:
    print(f'unknown udhcpc event "{event_type}"')
    sys.exit(1)
