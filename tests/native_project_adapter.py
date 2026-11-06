"""Unmodified project Git adapter invoking the native workflow implementation."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(os.environ['RUM_BINARY'])
with tempfile.TemporaryDirectory(prefix='rum-adapter-', dir='/private/tmp') as temp:
    root = Path(temp).resolve()
    home = root / 'd'
    config = root / 'gitconfig'
    config.write_text('[user]\n name = Test User\n email = test@example.test\n')
    env = dict(os.environ, GIT_CONFIG_GLOBAL=str(config), GIT_CONFIG_NOSYSTEM='1')
    def run(args, success=True):
        result = subprocess.run(list(map(str,args)), env=env, capture_output=True, text=True)
        assert (result.returncode == 0) == success, (args, result.stdout, result.stderr)
        return result
    def rum(*args):
        return json.loads(run([binary, '--home', home, *args]).stdout)['result']
    record = rum('workspace-init','--parent',root / 'projects','--name','native','--date','2026-11-06')
    workspace = Path(record['workspace'])
    code,meta = workspace/'code', workspace/'meta'
    run(['git','-C',code,'init','-q','--initial-branch=main'])
    (code/'source.txt').write_text('base\n')
    run(['git','-C',code,'add','source.txt'])
    run(['git','-C',code,'commit','-qm','Initial test commit'])
    entry = workspace/'rum.py'
    script = workspace/'.agents/skills/rum-project/scripts/git_adapter.py'
    result = json.loads(run([sys.executable,entry,'context']).stdout)['result']
    assert result['project_id'] == record['id']
    assert rum('doctor','--project',record['id'])['ok']
    def plan():
        return json.loads(run([sys.executable,script,'line-configure','--repo',code,'--store',meta,
                              '--line','mainline','--branch','main','--date','2026-11-06',
                              '--reason','Configure','--evidence','Inspected','--decision','Selected']).stdout)
    preview = plan()
    request = root/'request.json'
    request.write_text(json.dumps(preview['request']))
    result = json.loads(run([sys.executable,script,'integration-apply','--repo',code,'--store',meta,
                            '--request',request,'--expect-revision',preview['platform_preview']['revision'],
                            '--expect-code-revision',preview['code_revision']]).stdout)
    assert result['changed']
    state = json.loads((meta/'state.json').read_text())
    assert state['lines']['mainline']['extensions']['git']['branch'] == 'main'
    # The source project scripts are unchanged. Only their core transport differs.
    baseline = Path(os.environ['RUM_BASELINE'])/'resources/project-skill/scripts/git_adapter.py'
    assert hashlib.sha256(script.read_bytes()).digest() == hashlib.sha256(baseline.read_bytes()).digest()
    # Verify rejected stale outer context leaves the native snapshot unchanged.
    preview = plan()
    request.write_text(json.dumps(preview['request']))
    before = (meta/'state.json').read_bytes()
    run([sys.executable,script,'integration-apply','--repo',code,'--store',meta,
         '--request',request,'--expect-revision',preview['platform_preview']['revision'],
         '--expect-code-revision','stale'],success=False)
    assert before == (meta/'state.json').read_bytes()
print('Native workspace, Python project entry, unchanged direct Git adapter, native apply and stale-scope refusal: passed.')
