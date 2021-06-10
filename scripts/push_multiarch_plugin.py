#!/usr/bin/env python3
import hashlib
import argparse
import os
import tempfile
import gzip
import tarfile
import concurrent.futures

from docker_image import reference

from common import *

def tar_filter(p: Platform):
    def f(info: tarfile.TarInfo):
        # Ensure there's no directory called '/'
        if info.name == '':
            info.name = '.'

        # buildx messes up symlink targets...
        platform_prefix = f'/{p.dirname}'
        if info.issym() and info.linkname.startswith(platform_prefix):
            info.linkname = info.linkname[len(platform_prefix):]

        return info
    return f

def main():
    parser = argparse.ArgumentParser(description='Construct and push a multiarch Docker plugin')
    parser.add_argument('config', help='plugin config.json')
    parser.add_argument('rootfs', help='buildx rootfs parent directory')
    parser.add_argument('image', help='target image (registry/image:tag)')
    parser.add_argument('-p', '--platforms', default='linux/amd64', help='buildx platforms')

    args = parser.parse_args()

    platforms = [Platform(p) for p in args.platforms.split(',')]

    ref = reference.Reference.parse(args.image)
    hostname, repo = ref.split_hostname()
    tag = ref['tag']

    reg = DXF(hostname, repo, auth=dxf_auth)

    print(f'Pushing config file `{args.config}`')
    config_size = os.path.getsize(args.config)
    config_digest = reg.push_blob(filename=args.config, check_exists=True)
    print(f'Pushed config as {config_digest}')

    def push_platform(p):
        #with open(f'/tmp/{p.dirname}.tar.gz', mode='w+b') as f:
        with tempfile.TemporaryFile(mode='w+b', suffix='.tar.gz') as f:
            tar_name = f'{p.dirname}.tar'
            # Use gzip separately to force mtime=0 (deterministic gzipping!)
            with gzip.GzipFile(filename=tar_name, mode='w', fileobj=f, mtime=0) as gz:
                with tarfile.open(name=tar_name, mode='w', fileobj=gz) as tar:
                    path = os.path.join(args.rootfs, p.dirname)
                    print(f"tar'ing and gzip'ing {path}")
                    tar.add(path, arcname='', filter=tar_filter(p))
            f.seek(0, os.SEEK_SET)

            sha256 = hashlib.sha256()
            for chunk in iter(lambda: f.read(8192), b''):
                sha256.update(chunk)
            h = 'sha256:' + sha256.hexdigest()

            layer_size = f.tell()
            f.seek(0, os.SEEK_SET)

            print(f'Pushing {p.buildx} layer')
            layer_digest = reg.push_blob(data=f, digest=h, check_exists=True)
            print(f'Pushed {p.buildx} layer as {layer_digest}')

            platform_tag = p.tag(tag)
            print(f'Pushing {p.buildx} manifest with tag {platform_tag}')
            size, digest = reg.push_manifest({
                'schemaVersion': 2,
                'mediaType': MTYPE_MANIFEST,
                'config': {
                    'mediaType': MTYPE_PLUGIN_CONFIG,
                    'size': config_size,
                    'digest': config_digest,
                },
                'layers': [
                    {
                        'mediaType': MTYPE_LAYER,
                        'size': layer_size,
                        'digest': layer_digest,
                    }
                ],
            }, ref=platform_tag)
            print(f'Pushed {p.buildx} manifest with digest {digest}')

            return size, digest

    mf_list = {
        'schemaVersion': 2,
        'mediaType': MTYPE_MANIFEST_LIST,
        'manifests': [],
    }
    with concurrent.futures.ThreadPoolExecutor() as executor:
        fs = {executor.submit(push_platform, p): p for p in platforms}

        for f in concurrent.futures.as_completed(fs):
            p = fs[f]
            try:
                size, digest = f.result()
            except Exception as ex:
                print(f'Exception pushing `{p.buildx}`: {ex}')
                continue

            mf_list['manifests'].append({
                'mediaType': MTYPE_MANIFEST,
                'size': size,
                'digest': digest,
                'platform': p.manifest,
            })

    print(f'Pushing {args.image} manifest list')
    reg.push_manifest(mf_list, ref=tag, mime=MTYPE_MANIFEST_LIST)
    print(f'Pushed {args.image}')

if __name__ == '__main__':
    main()
