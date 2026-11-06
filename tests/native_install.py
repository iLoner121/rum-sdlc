"""Native installation, lease and legacy recovery acceptance without Python on PATH.
Python is the test driver only. Every product subprocess executes the Go binary.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

binary=Path(os.environ.get('RUM_NATIVE_BINARY','./rum')).resolve()
version=subprocess.check_output([str(binary),'--version'],text=True).strip()
assert version=='0.9.0'
with tempfile.TemporaryDirectory(prefix='rum-install-',dir='/private/tmp') as temp:
    root=Path(temp); home=root/'home'; commands=root/'commands'; release=root/'release'
    env=dict(os.environ,PATH='',RUMSDLC_HOME=str(home))
    def run(*args,entry=binary,ok=True):
        p=subprocess.run([str(entry),'--home',str(home),*args],env=env,capture_output=True,text=True,timeout=30)
        assert (p.returncode==0)==ok,(args,p.stdout,p.stderr)
        return p
    def install(destination=release,*extra,ok=True):
        return run('install','--destination',str(destination),'--bin-dir',str(commands),*extra,ok=ok)
    value=json.loads(install().stdout)
    assert value['program_ready'] and not value['service_started']
    assert not (home/'registry.json').exists() and not list(home.glob('operation-*.lock'))
    entry=commands/'rum'
    assert run('--version',entry=entry).stdout.strip()==version
    assert json.loads(run('list',entry=entry).stdout)['result']['counts']['local']==0
    with (home/'install.lock').open('a') as gate:
        fcntl.flock(gate,fcntl.LOCK_EX|fcntl.LOCK_NB)
        assert 'installation is in progress' in run('--version',entry=entry,ok=False).stderr
        install(root/'second',ok=False)
    assert not (root/'second').exists()
    # Offline upgrade must not inspect registry or projects.
    (home/'registry.json').write_bytes(b'corrupt registry intentionally preserved')
    old_entry=entry.read_bytes()
    install(root/'second')
    assert (home/'registry.json').read_bytes()==b'corrupt registry intentionally preserved'
    assert entry.read_bytes()==old_entry
    (home/'registry.json').unlink()
    # A live native daemon holds the installation lease; upgrade leaves it running.
    run('daemon','start','--interval','5','--discovery-interval','5',entry=entry)
    pid=json.loads((home/'service.json').read_text())['pid']
    try:
        assert json.loads(run('daemon','status',entry=entry).stdout)['result']['running']
        install(root/'third',ok=False)
        assert json.loads((home/'service.json').read_text())['pid']==pid
    finally:
        run('daemon','stop',entry=entry)
    for _ in range(100):
        if not (home/'service.sock').exists():break
        time.sleep(.05)
    install(root/'third')
    # Explicit legacy rollback resumes the native daemon after durable restoration.
    target=root/'legacy-resource';target.write_bytes(b'after')
    journal={'format':'rum-install-transaction/1','home':str(home),'phase':'applying',
             'updates':[{'path':str(target),'before':{'data':b'before'.hex(),'mode':0o600},
                         'after':{'data':b'after'.hex(),'mode':0o644}}],
             'stores':[],'lock_owner':None,'was_running':True,
             'previous_runtime':'legacy.py','runtime':'legacy.py',
             'settings':{'interval':5,'discovery_interval':5}}
    (home/'install-transaction.json').write_text(json.dumps(journal))
    assert '--recover-only' in install(root/'fourth',ok=False).stderr
    result=json.loads(install(root/'fourth','--recover-only').stdout)
    assert result['outcome']=='rolled-back' and target.read_bytes()==b'before'
    try:
        for _ in range(100):
            status=json.loads(run('daemon','status',entry=entry).stdout)['result']
            if status['running']:break
            time.sleep(.05)
        assert status['running']
        assert not (home/'install-transaction.json').exists()
    finally:
        run('daemon','stop',entry=entry)
    for _ in range(100):
        if not (home/'service.sock').exists():break
        time.sleep(.05)
    assert json.loads(install(root/'fourth','--recover-only').stdout)['recovered'] is False
    # Source runtime verification must reject an altered immutable release.
    (release/'rum').write_bytes(b'changed release')
    assert 'differs' in install(ok=False).stderr
print('PASS: native offline installation, immutable releases, global launcher, concurrent leases, service preservation, legacy rollback/restart; product PATH empty')
