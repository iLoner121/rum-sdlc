"""Private adapter state; never consumed by RumSDLC and never stored in code Git."""
from contextlib import contextmanager
import fcntl
import hashlib
import json
import os
from pathlib import Path
from store import atomic

_home = None

def configure(value=None):
    global _home
    _home = Path(value or os.environ.get('RUMSDLC_AGENT_HOME', str(Path.home() / '.rumsdlc/agent-state/git'))).expanduser().absolute()
    if any(p.is_symlink() for p in [_home, *_home.parents]):
        raise ValueError('Adapter state path cannot contain symlinks')
    _home = _home.resolve()
    return _home


def path(identity, name, repository=None):
    home = _home or configure()
    identity = Path(identity).resolve()
    roots = [identity, Path(repository).resolve()] if repository else [identity]
    if any(home.is_relative_to(p) or p.is_relative_to(home) for p in roots):
        raise ValueError('Adapter state must be outside code/worktree/Git directories')
    key = hashlib.sha256(os.fsencode(str(identity))).hexdigest()
    target = home / key / name
    if any(p.is_symlink() for p in [target, *target.parents]):
        raise ValueError('Adapter state path contains a symlink')
    return target


@contextmanager
def locked():
    home = _home or configure()
    home.mkdir(parents=True, exist_ok=True, mode=0o700)
    with (home / 'adapter.lock').open('a') as out:
        try:
            fcntl.flock(out, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError('Another adapter operation is active; retry after it finishes')
        try:
            yield
        finally:
            fcntl.flock(out, fcntl.LOCK_UN)


def migrate(legacy_index, legacy_focus, index_dest, focus_dest, validate_index, validate_focus):
    # Explicit import only. Legacy originals are retained for separately reviewed cleanup.
    planned = []
    for old, new, validator in [(legacy_index, index_dest, validate_index), (legacy_focus, focus_dest, validate_focus)]:
        if old.exists():
            if any(p.is_symlink() for p in [old, *old.parents]):
                raise ValueError('Legacy adapter state contains a symlink')
            value = validator(json.loads(old.read_text()))
            if new.exists() and json.loads(new.read_text()) != value:
                raise ValueError('External state differs; preserve both and reconcile explicitly')
            planned.append((new, value))
    for new, value in planned:
        new.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        atomic(new, value)
    return {'imported': [str(p) for p, _ in planned], 'legacy_retained': True,
            'next': 'Verify external state and inventory all worktrees before separately authorized legacy cleanup'}
