"""Run with RUM_BINARY=/path/to/new/rum python3 tests/topic_numbering.py.
Exercise short references in a fresh isolated workspace and real Git adapter.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(os.environ['RUM_BINARY']).resolve()
with tempfile.TemporaryDirectory(prefix='rum-numbering-', dir='/private/tmp') as temp:
    root = Path(temp).resolve()
    home = root/'device'
    config = root/'gitconfig'
    config.write_text('[user]\n name = Test User\n email = test@example.test\n')
    env = dict(os.environ, GIT_CONFIG_GLOBAL=str(config), GIT_CONFIG_NOSYSTEM='1')

    def run(args, success=True):
        r = subprocess.run(list(map(str, args)), env=env, text=True, capture_output=True)
        assert (r.returncode == 0) == success, (args, r.stdout, r.stderr)
        return r

    def rum(*args):
        return json.loads(run([binary, '--home', home, *args]).stdout)['result']

    project = rum('workspace-init', '--parent', root/'projects', '--name', 'numbering', '--date', '2026-11-10')
    workspace = Path(project['workspace'])
    meta, code = workspace/'meta', workspace/'code'
    scripts = workspace/'.agents/skills/rum-project/scripts'

    def flow(*args, success=True):
        r = run([sys.executable, scripts/'workflow.py', *args, '--store', meta], success)
        return json.loads(r.stdout) if success else r

    def apply(action, params):
        path = root/'request.json'
        path.write_text(json.dumps(dict(action=action, params=params, date='2026-11-10', reason='Fixture', evidence='Fixture', decision='Fixture')))
        preview = flow('command', '--request', path, '--dry-run')
        result = flow('command', '--request', path, '--expect-revision', preview['revision'])
        assert result.get('short_id') == preview.get('short_id')
        return result

    first = apply('topic.create', dict(name='First', type='feat', slug='export-report', unassigned_reason='Planned'))
    assert first['short_id'] == 'T1' and first['result'].startswith('T-')
    canonical = first['result']
    before = (meta/'state.json').read_bytes()
    assert flow('query', '--view', 'show', '--table', 'topics', '--id', 'T1') == flow('query', '--view', 'show', '--table', 'topics', '--id', canonical)
    info = flow('document', '--table', 'topics', '--id', canonical, '--field', 'context', '--ensure')
    assert flow('document', '--table', 'topics', '--id', 'T1', '--field', 'context')['path'] == info['path']
    assert (meta/'state.json').read_bytes() == before
    for invalid in ['T0', 'T01', 'T-1', 'T999']:
        flow('query', '--view', 'show', '--table', 'topics', '--id', invalid, success=False)
    apply('topic.state', dict(id='T1', status='active'))
    task = apply('task.create', dict(topic_id='T1', name='Design', kind='design'))
    assert task['short_id'] == 'T1/t001'
    apply('task.state', dict(id='T1/t001', status='active'))
    apply('task.state', dict(id='T1/t001', status='done', result='Fixture complete'))
    assert canonical in flow('document', '--table', 'tasks', '--id', 'T1/t001', '--ensure')['path']
    run(['git', '-C', code, 'init', '-q', '--initial-branch=main'])
    (code/'source.txt').write_text('fixture\n')
    run(['git', '-C', code, 'add', 'source.txt'])
    run(['git', '-C', code, 'commit', '-qm', 'Initial fixture'])

    def git_tool(*args):
        return json.loads(run([sys.executable, scripts/'git_adapter.py', *args, '--repo', code, '--store', meta]).stdout)

    preview = git_tool('line-configure', '--line', 'mainline', '--branch', 'main', '--date', '2026-11-10', '--reason', 'Fixture', '--evidence', 'Fixture', '--decision', 'Fixture')
    path = root/'integration.json'
    path.write_text(json.dumps(preview['request']))
    git_tool('integration-apply', '--request', path, '--expect-revision', preview['platform_preview']['revision'], '--expect-code-revision', preview['code_revision'])
    args = ['add', '--path', root/'topic-tree', '--topic', 'T1', '--first-code', '--code-history-check', 'Fresh fixture; no prior Topic code']
    preview = git_tool(*args, '--dry-run')
    assert 'feat/T1-export-report' in json.dumps(preview), preview
    custom = git_tool(*args, '--branch', 'feat/custom-name', '--dry-run')
    assert 'feat/custom-name' in json.dumps(custom), custom
    git_tool(*args, '--expect-revision', preview['revision'])
    rum('bind-code', '--project', project['id'], '--topic', 'T1', '--branch', 'feat/T1-export-report')
    binding = json.loads((meta/'project-config.json').read_text())['repositories']['code']['topic_branches']
    assert binding[canonical] == 'feat/T1-export-report' and 'T1' not in binding
    # Restore the registered branch without renaming or recreating Topic code.
    run(['git','-C',code,'worktree','remove',root/'topic-tree'])
    restored = git_tool('add','--path',root/'restored-tree','--topic','T1','--dry-run')
    assert 'feat/T1-export-report' in json.dumps(restored), restored
    assert flow('check')['ok']
print('Passed: fresh workspace, CLI/Task/document aliases, old identity, Git defaults/override, canonical association and state check.')
