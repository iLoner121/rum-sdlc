"""Run with RUM_BINARY=/path/to/new/rum python3 tests/topic_numbering.py.
Exercise short references and parallel worktrees in an isolated real repository.
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

    def git_tool(*args, repo=code, store=meta, success=True):
        params = ['--store', store] if store is not None else []
        result = run([sys.executable, scripts/'git_adapter.py', *args, '--repo', repo, *params], success)
        return json.loads(result.stdout or result.stderr)

    def apply_integration(preview):
        path = root/'integration.json'
        path.write_text(json.dumps(preview['request']))
        git_tool('integration-apply', '--request', path, '--expect-revision', preview['platform_preview']['revision'], '--expect-code-revision', preview['code_revision'])

    line_args = ['--date', '2026-11-10', '--reason', 'Fixture', '--evidence', 'Fixture', '--decision', 'Fixture']
    apply_integration(git_tool('line-configure', '--line', 'mainline', '--branch', 'main', *line_args))
    args = ['add', '--path', root/'topic-tree', '--topic', 'T1', '--first-code', '--code-history-check', 'Fresh fixture; no prior Topic code']
    preview = git_tool(*args, '--dry-run')
    assert 'feat/T1-export-report' in json.dumps(preview), preview
    custom = git_tool(*args, '--branch', 'feat/custom-name', '--dry-run')
    assert 'feat/custom-name' in json.dumps(custom), custom
    git_tool(*args, '--expect-revision', preview['revision'])
    rum('bind-code', '--project', project['id'], '--topic', 'T1', '--branch', 'feat/T1-export-report')
    binding = json.loads((meta/'project-config.json').read_text())['repositories']['code']['topic_branches']
    assert binding[canonical] == 'feat/T1-export-report' and 'T1' not in binding
    apply('line.create', dict(id='parallel', name='Parallel', type='development'))
    run(['git', '-C', code, 'branch', 'line/parallel'])
    apply_integration(git_tool('line-configure', '--line', 'parallel', '--branch', 'line/parallel', '--base-ref', 'main', *line_args))
    apply('topic.create', dict(name='Second', type='feat', slug='parallel-work', line_id='parallel', unassigned_reason='Fixture'))
    apply('topic.state', dict(id='T2', status='active'))
    first_tree, second_tree = root/'topic-tree', root/'second-tree'
    second_add = ['add', '--path', second_tree, '--topic', 'T2', '--first-code', '--code-history-check', 'Fresh fixture; no prior Topic code']
    git_tool(*second_add, '--expect-revision', git_tool(*second_add, '--dry-run')['revision'])
    (first_tree/'source.txt').write_text('first change\n')
    (second_tree/'second.txt').write_text('second change\n')

    def inventory(command='list', **kwargs):
        report = git_tool(command, **kwargs)
        return report, {row['path']: row for row in report['worktrees']}

    apply('topic.state', dict(id='T1', status='paused'))
    assert git_tool('current', repo=second_tree)['short_id'] == 'T2'
    assert git_tool('current', repo=first_tree)['short_id'] == 'T1'
    report, rows = inventory('check')
    first_row, second_row = rows[str(first_tree)], rows[str(second_tree)]
    assert report['ok'] and rows[str(code)]['primary'] and not first_row['primary']
    assert first_row['topic']['short_id'] == 'T1' and first_row['topic']['status'] == 'paused'
    assert first_row['line_id'] == 'mainline' and first_row['target_branch'] == 'main'
    assert second_row['topic']['short_id'] == 'T2' and second_row['target_branch'] == 'line/parallel'
    assert first_row['dirty'] is True and second_row['dirty'] is True
    assert (first_tree/'source.txt').read_text() == 'first change\n'
    _, local_only = inventory(store=None)
    assert local_only[str(first_tree)]['binding'] and local_only[str(first_tree)]['topic'] is None

    ref = 'feat/T1-export-report'
    sha = run(['git', '-C', first_tree, 'rev-parse', 'HEAD']).stdout.strip()
    remove = ['remove', '--path', first_tree, '--expect-head', sha, '--resources-check', 'No processes in disposable fixture']
    assert 'Preserve tracked' in git_tool(*remove, '--dry-run', store=None, success=False)['error']
    run(['git', '-C', second_tree, 'switch', '-c', 'wrong-branch'])
    report, rows = inventory('check', success=False)
    stale = rows[str(second_tree)]
    assert not report['ok'] and stale['topic'] is None and stale['dirty'] is True
    assert 'binding branch is stale' in stale['issues'] and stale['next_steps']
    run(['git', '-C', second_tree, 'switch', 'feat/T2-parallel-work'])

    hidden = second_tree.rename(root/'temporarily-moved')
    try:
        report, rows = inventory()
        missing = rows[str(second_tree)]
        assert not report['ok'] and missing['dirty'] is None
        assert 'workspace unavailable' in missing['issues'] and missing['next_steps']
        assert rows[str(first_tree)]['topic']['short_id'] == 'T1'
    finally:
        hidden.rename(second_tree)

    apply('topic.state', dict(id='T1', status='active'))
    run(['git', '-C', first_tree, 'add', 'source.txt'])
    run(['git', '-C', first_tree, 'commit', '-qm', 'Deliver first fixture change'])
    run(['git', '-C', code, 'merge', '--ff-only', ref])
    delivered = run(['git', '-C', code, 'rev-parse', 'HEAD']).stdout.strip()
    delivery = git_tool('delivery', '--topic', 'T1', '--commit', delivered)
    assert delivery['source_history_preserved']
    apply('topic.state', dict(id='T1', status='review', verification='Fixture Git checks'))
    apply('topic.state', dict(id='T1', status='ready', acceptance='Fixture accepted'))
    close_request = root/'close.json'
    close_request.write_text(json.dumps(dict(action='topic.close', params=dict(
        id=canonical, outcome='delivered', summary='Fixture delivered', verification='Fixture Git checks',
        delivery_record=delivered, extensions={'git': {'integration_commit': delivered, 'retained_ref': None}}),
        date='2026-11-10', reason='Fixture', evidence='Fixture', decision='Fixture')))
    before = (meta/'state.json').read_bytes()
    refusal = git_tool('closure-plan', '--request', close_request, success=False)
    assert 'planned or active Milestone' in refusal['error'], refusal
    assert (meta/'state.json').read_bytes() == before
    milestone = apply('milestone.create', dict(name='First release'))['result']
    apply('relation.move', dict(kind='milestone', topic_id='T1', to=milestone))
    apply_integration(git_tool('closure-plan', '--request', close_request))
    _, rows = inventory('check')
    assert rows[str(first_tree)]['topic']['status'] == 'closed' and rows[str(first_tree)]['next_steps']
    remove = ['remove', '--path', first_tree, '--expect-head', delivered, '--resources-check', 'No processes in disposable fixture']
    preview = git_tool(*remove, '--dry-run', store=None)
    assert git_tool(*remove, '--expect-revision', preview['revision'], store=None)['changed']
    assert not first_tree.exists()
    assert run(['git', '-C', code, 'rev-parse', ref]).stdout.strip() == delivered
    assert git_tool('current', repo=second_tree)['short_id'] == 'T2'
    assert (second_tree/'second.txt').read_text() == 'second change\n'
    _, rows = inventory('check')
    assert str(first_tree) not in rows and rows[str(second_tree)]['dirty'] is True

    # Restore a registered branch of the remaining open Topic without recreating code.
    (second_tree/'second.txt').unlink()
    run(['git', '-C', code, 'worktree', 'remove', second_tree])
    restored = git_tool('add', '--path', root/'restored-tree', '--topic', 'T2', '--dry-run')
    assert restored['plan']['branch'] == 'feat/T2-parallel-work' and not restored['plan']['new_branch']
    assert flow('check')['ok']
print('Passed: short references, parallel inspection/resume, missing and stale diagnostics, isolated delivery/removal, and branch recovery.')
