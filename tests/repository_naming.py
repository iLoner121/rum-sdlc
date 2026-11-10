"""Native CLI naming preview: RUM_BINARY=/path/to/rum python3 tests/repository_naming.py.
All operations are local; no GitHub repository is created or renamed.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile

binary = Path(os.environ['RUM_BINARY']).resolve()
with tempfile.TemporaryDirectory(prefix='rum-repo-name-') as temp:
    root = Path(temp).resolve()
    home = root/'device'

    def run(*args, success=True):
        r = subprocess.run([str(binary), '--home', str(home), *map(str,args)], text=True, capture_output=True)
        assert (r.returncode == 0) == success, (args,r.stdout,r.stderr)
        return json.loads(r.stdout)['result'] if success else r

    project = run('workspace-init','--parent',root/'projects','--name','display-name','--date','2026-11-10')
    workspace = Path(project['workspace'])
    state = workspace/'meta/state.json'
    before = state.read_bytes()
    value = run('repository-plan','--project',project['id'])
    assert value['repository_name'] == 'RumSDLC_display-name' and value['source'] == 'project-name'
    run('bind-code','--project',project['id'],'--repository','https://github.com/example/source-app.git','--kind','git')
    value = run('repository-plan','--project',project['id'])
    assert value['repository_name'] == 'RumSDLC_source-app' and value['source'] == 'code-repository'
    value = run('repository-plan','--project',project['id'],'--name','edited-name')
    assert value['repository_name'] == 'RumSDLC_edited-name' and value['source'] == 'user' and value['editable']
    run('repository-plan','--project',project['id'],'--name','bad/name',success=False)
    assert state.read_bytes() == before
    assert run('doctor','--project',project['id'])['ok']
print('Passed: actual CLI project/code/custom naming priority, editable preview, invalid-name rejection, no business-state mutation.')
