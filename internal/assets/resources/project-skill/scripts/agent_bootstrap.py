"""Locate the installed core from managed context, or from the release package."""
import json
import os
from pathlib import Path
import sys
sys.dont_write_bytecode = True
for parent in Path(__file__).resolve().parents:
    context = parent / '.rum-context.json'
    if context.is_file():
        if context.is_symlink() or (parent / '.rum-agent-update.json').exists():
            raise ValueError('Unsafe context or interrupted project update; recover with agent-install')
        cfg = json.loads(context.read_text())
        if not isinstance(cfg, dict) or not isinstance(cfg.get('device_home'), str) or not Path(cfg['device_home']).is_absolute():
            raise ValueError('Invalid project device context')
        # Project identity is authoritative; shell variables cannot select another device.
        os.environ['RUMSDLC_HOME'] = str(Path(cfg['device_home']).resolve())
        core = Path(cfg['core_scripts'])
        os.environ['RUMSDLC_AGENT_HOME'] = str(Path(os.environ['RUMSDLC_HOME']) / 'agent-state/git')
        break
    core = parent / 'runtime/scripts'
    if (core / 'store.py').is_file():
        break
else:
    raise ValueError('Core runtime missing; reinstall and regenerate project skill')
if not (core / 'store.py').is_file():
    raise ValueError('Core runtime missing; reinstall and regenerate project skill')
sys.path.insert(0, str(core))

# Raw package scripts need an explicit device; the CLI supplies it automatically.
if not os.environ.get('RUMSDLC_HOME'):
    raise ValueError('No device context; use rum tool or set RUMSDLC_HOME explicitly')

# Hold the runtime gate for direct managed-script execution, too.
if os.environ.get('RUMSDLC_HOME'):
    import fcntl
    import atexit
    _home = Path(os.environ['RUMSDLC_HOME']).expanduser().resolve()
    _home.mkdir(parents=True, exist_ok=True, mode=0o700)
    if (_home / 'install.lock').is_symlink():
        raise ValueError('Runtime gate cannot be a symlink')
    _gate = (_home / 'install.lock').open('a')
    try:
        fcntl.flock(_gate, fcntl.LOCK_SH | fcntl.LOCK_NB)
        if (_home / 'install-transaction.json').exists():
            raise ValueError('Pending installation recovery; run installer --recover-only')
    except BaseException:
        _gate.close()
        raise
    atexit.register(_gate.close)


def check_store(value):
    """A managed Store cannot be accessed under a different device's lease."""
    if not value:
        return
    store = Path(value).expanduser().resolve()
    context = store.parent / '.rum-context.json'
    if context.exists():
        cfg = json.loads(context.read_text())
        if (not isinstance(cfg, dict) or not isinstance(cfg.get('device_home'), str)
                or Path(cfg['device_home']).resolve() != _home
                or not isinstance(cfg.get('meta'), str) or Path(cfg['meta']).resolve() != store):
            raise ValueError('Store belongs to another project/device context; use its project entry')
        if (store.parent / '.rum-agent-update.json').exists():
            raise ValueError('Interrupted project update; recover with agent-install')
