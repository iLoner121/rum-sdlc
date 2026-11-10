"""Real 0.8.5 Python/Go releases upgrade to 0.9.0 without implicit project edits."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary=Path(os.environ['RUM_NATIVE_BINARY']).resolve()
previous=Path(os.environ['RUM_PREVIOUS_BINARY']).resolve()
baseline=Path(os.environ['RUM_BASELINE']).resolve()
env=dict(os.environ,PYTHONDONTWRITEBYTECODE='1')

def run(argv):
    return subprocess.run(list(map(str,argv)),env=env,capture_output=True,text=True,check=True,timeout=30).stdout

def snapshot(root):
    return {str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in root.rglob('*') if p.is_file()}

current_version=json.loads(run([binary,'version']))['result']['version']
current_display=run([binary,'--version']).strip()
assert current_version
assert run([previous,'--version']).strip()=='0.8.5'
with tempfile.TemporaryDirectory(prefix='rum-v090-',dir='/private/tmp') as folder:
    for kind in ['python','go']:
        root=Path(folder)/kind;home=root/'device';commands=root/'bin';old=root/'releases/0.8.5';new=root/('releases/'+current_version)
        if kind=='python':
            run([sys.executable,'-B',baseline/'install.py','--home',home,'--destination',old,'--bin-dir',commands])
            old_entry=[sys.executable,'-B',old/'rum.py']
        else:
            run([previous,'--home',home,'install','--destination',old,'--bin-dir',commands])
            old_entry=[old/'rum']
        record=json.loads(run([*old_entry,'--home',home,'workspace-init','--parent',root/'projects','--name','retained','--date','2026-11-06']))['result']
        workspace=Path(record['workspace']);state=(workspace/'meta/state.json').read_bytes()
        before=snapshot(workspace);old_release=snapshot(old);registry=(home/'registry.json').read_bytes()
        installation=json.loads(run([binary,'--home',home,'install','--destination',new,'--bin-dir',commands]))
        assert installation['version']==current_version
        assert run([commands/'rum','--version']).strip()==current_display
        assert json.loads((home/'active.json').read_text())['version']==current_version
        assert json.loads((new/'MANIFEST.json').read_text())['version']==current_version
        assert snapshot(workspace)==before and snapshot(old)==old_release
        assert (home/'registry.json').read_bytes()==registry
        def call(*args):return json.loads(run([commands/'rum',*args]))['result']
        health=call('doctor','--project',record['id'])
        compatibility=health['project_compatibility']
        assert health['ok'] and compatibility['rules_version']=='0.8.5'
        assert compatibility['runtime_version']==current_version and compatibility['update_available']
        preview=call('project-update','--project',record['id'],'--dry-run')
        assert preview['rules_version']==current_version and snapshot(workspace)==before
        call('project-update','--project',record['id'],'--expect-revision',preview['revision'])
        assert json.loads((workspace/'.rum-context.json').read_text())['version']==current_version
        assert json.loads((workspace/'meta/project-config.json').read_text())['rules_version']==current_version
        assert (workspace/'meta/state.json').read_bytes()==state
        health=call('doctor','--project',record['id'])
        assert health['ok'] and not health['project_compatibility']['update_available']
        assert health['project_compatibility']['runtime_gate_ready']
        assert run([*old_entry,'--version']).strip()=='0.8.5'
        assert snapshot(old)==old_release
        assert run([commands/'rum','tool','workflow','--','--version']).strip()=='4.1.2'
        fresh=call('workspace-init','--parent',str(root/'projects'),'--name','new','--date','2026-11-06')
        assert json.loads((Path(fresh['workspace'])/'.rum-context.json').read_text())['version']==current_version
        assert call('doctor','--project',fresh['id'])['ok']
        print('PASS:',kind,'0.8.5 -> 0.9.0; old release/project preserved, explicit update and new project use 0.9.0, state protocol remains 4.1.2')
