"""Atomic project JSON snapshots. Documents are separate; this module never runs Git."""
import sys
sys.dont_write_bytecode = True
from contextlib import contextmanager
from copy import deepcopy
import hashlib
import json
import os
from pathlib import Path
import socket
import uuid
import validation as v


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(',', ':')).encode()).hexdigest()


def safe(root, relative):
    root = Path(root).absolute()
    path = root / relative
    v.need(path.resolve().is_relative_to(root.resolve()), 'Path escapes workflow root')
    for part in [path, *path.parents]:
        v.need(not part.is_symlink(), 'Symlink is not allowed: ' + str(part))
        if part == root: break
    return path


def atomic(path, data):
    temporary = path.with_name(path.name + '.' + uuid.uuid4().hex + '.tmp')
    try:
        with temporary.open('x', encoding='utf-8') as out:
            json.dump(data, out, ensure_ascii=False, indent=2); out.write('\n'); out.flush(); os.fsync(out.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists(): temporary.unlink()


from native_bridge import call

class Store:
    def __init__(self, root):
        self.root = Path(root).expanduser().absolute()
        safe(self.root, '')
        self.path = safe(self.root, 'state.json')
        self.lock = safe(self.root, '.workflow.lock')

    def read(self, optional=False):
        return call('read', root=str(self.root), optional=optional)

    def execute(self, request, dry_run=False, expect=None):
        return call('execute', root=str(self.root), request=request,
                    dry_run=dry_run, expect=expect)

    def lock_status(self):
        return call('lock-status', root=str(self.root))

    def recover_lock(self, expected, confirmed_idle):
        return call('lock-recover', root=str(self.root), expected=expected,
                    confirmed_idle=confirmed_idle)

    def document(self, table, identifier, field='description', content=None,
                 expected_hash=None, ensure=False, correction=None):
        return call('document', root=str(self.root), table=table, identifier=identifier,
                    field=field, content=content, expected_hash=expected_hash,
                    ensure=ensure, correction=correction)
