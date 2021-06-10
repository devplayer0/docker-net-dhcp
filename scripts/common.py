import json
import os

import dxf

MTYPE_PLUGIN_CONFIG = 'application/vnd.docker.plugin.v1+json'
MTYPE_LAYER = 'application/vnd.docker.image.rootfs.diff.tar.gzip'
MTYPE_MANIFEST = 'application/vnd.docker.distribution.manifest.v2+json'
MTYPE_MANIFEST_LIST = 'application/vnd.docker.distribution.manifest.list.v2+json'

class Platform:
    def __init__(self, s):
        self.buildx = s

        split = s.split('/')
        if len(split) < 2:
            raise Exception('Invalid platform format')

        self.dirname = '_'.join(split)

        self.os = split[0]
        self.architecture = split[1]

        self.variant = None
        if len(split) > 3:
            raise Exception('Invalid platform format')
        elif len(split) == 3:
            self.variant = split[2]
        elif self.architecture == 'arm64':
            # Weird exception? (seen in alpine images)
            self.variant = 'v8'

    @property
    def manifest(self):
        d = {
            'os': self.os,
            'architecture': self.architecture,
        }
        if self.variant is not None:
            d['variant'] = self.variant

        return d

    def tag(self, t):
        if self.variant is not None:
            return f'{t}-{self.os}-{self.architecture}-{self.variant}'
        return f'{t}-{self.os}-{self.architecture}'

    def __str__(self):
        return f'Platform(os={self.os}, architecture={self.architecture}, variant={self.variant})'
    def __repr__(self):
        return str(self)

def dxf_auth(reg: dxf.DXF, res):
    reg.authenticate(username=os.getenv('REGISTRY_USERNAME'), password=os.getenv('REGISTRY_PASSWORD'), response=res)

class DXF(dxf.DXF):
    def set_manifest(self, alias, manifest_json, mime=MTYPE_MANIFEST):
        """
        Give a name (alias) to a manifest.

        :param alias: Alias name
        :type alias: str

        :param manifest_json: A V2 Schema 2 manifest JSON string
        :type digests: list
        """
        self._request('put',
                      'manifests/' + alias,
                      data=manifest_json,
                      headers={'Content-Type': mime})

    def push_manifest(self, manifest_dict, ref=None, mime=MTYPE_MANIFEST):
        mf = json.dumps(manifest_dict, sort_keys=True).encode('utf-8')
        size = len(mf)
        digest = dxf.hash_bytes(mf)

        if ref is None:
            ref = digest

        self.set_manifest(ref, mf, mime=mime)
        return size, digest
