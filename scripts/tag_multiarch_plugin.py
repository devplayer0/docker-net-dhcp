#!/usr/bin/env python3
import argparse

from docker_image import reference

from common import *

def main():
    parser = argparse.ArgumentParser(description='Re-tag an existing multiarch plugin')
    parser.add_argument('image', help='existing image (registry/image:tag)')
    parser.add_argument('tag', help='new tag')
    parser.add_argument('-p', '--platforms', default='linux/amd64', help='buildx platforms')

    args = parser.parse_args()

    platforms = [Platform(p) for p in args.platforms.split(',')]

    ref = reference.Reference.parse(args.image)
    hostname, repo = ref.split_hostname()
    without_tag = ref['name']

    old_tag = ref['tag']
    new_tag = args.tag

    reg = DXF(hostname, repo, auth=dxf_auth)

    for p in platforms:
        mf = reg.get_manifest(p.tag(old_tag))

        print(f'Re-tagging {without_tag}:{p.tag(old_tag)} as {without_tag}:{p.tag(new_tag)}')
        reg.set_manifest(p.tag(new_tag), mf, mime=MTYPE_MANIFEST)

    print(f'Re-tagging {args.image} as {without_tag}:{new_tag}')
    mf = reg.get_manifest(old_tag)
    reg.set_manifest(new_tag, mf, mime=MTYPE_MANIFEST_LIST)

if __name__ == '__main__':
    main()
