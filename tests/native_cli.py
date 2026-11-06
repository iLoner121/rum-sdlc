"""End-to-end core parity through two real executables and independent stores."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

baseline = Path(os.environ['RUM_BASELINE'])
binary = Path(os.environ['RUM_WORKFLOW_BINARY'])
with tempfile.TemporaryDirectory(prefix='rum-native-cli-', dir='/tmp') as temp:
    root = Path(temp)
    old, new = root / 'python', root / 'go'
    def run(native, args, raw=None, success=True):
        prefix = [str(binary)] if native else [sys.executable, '-B', str(baseline / 'runtime/scripts/workflow.py')]
        # Native operations must work with no Python, Go, Git or shell on PATH.
        env = dict(os.environ)
        if native:
            env['PATH'] = ''
        p = subprocess.run(prefix + args, input=raw, capture_output=True, text=True, env=env)
        assert (p.returncode == 0) == success, (native, args, p.stdout, p.stderr)
        return json.loads(p.stdout if success else p.stderr)
    def command(action, **params):
        request = dict(action=action, params=params, date='2026-11-06', reason='Parity', evidence='Evidence', decision='Decision')
        raw = json.dumps(request, ensure_ascii=False)
        outputs = []
        for native, folder in ((False, old), (True, new)):
            args = ['command', '--store', str(folder), '--request', '-']
            pre = run(native, args + ['--dry-run'], raw)
            result = run(native, args + ['--expect-revision', pre['revision']], raw)
            outputs.append((pre, result))
        assert outputs[0] == outputs[1], (action, outputs)
        assert json.loads((old / 'state.json').read_text()) == json.loads((new / 'state.json').read_text()), action
        return outputs[1][1]['result']
    command('project.init', id='P-parity', name='跨平台 😀')
    milestone = command('milestone.create', name='First')
    command('milestone.activate', id=milestone)
    topic = command('topic.create', name='功能', type='feat', slug='feature', milestone_id=milestone)
    iteration = command('workset.create', name='Round')
    command('workset.propose', id=iteration, topics=[topic])
    command('workset.activate', id=iteration)
    command('topic.state', id=topic, status='active')
    task = command('task.create', topic_id=topic, name='Implement', kind='implement')
    command('task.state', id=task, status='active')
    command('task.state', id=task, status='done', result='Passed')
    command('topic.state', id=topic, status='review', verification='Test report')
    command('topic.state', id=topic, status='ready', acceptance='Accepted')
    command('topic.close', id=topic, outcome='delivered', summary='Delivered', verification='Checked', delivery_record='docs/result')
    command('workset.close', id=iteration, outcome='completed', summary='Done', results=[dict(topic=topic, disposition='completed', note='Delivered', evidence='Report')])
    command('milestone.close', id=milestone, outcome='completed', summary='Accepted')
    command('project.close', summary='Finished')
    command('task.correct', id=task, result='Detailed result')
    command('extension.set', table='project', id='P-parity', namespace='example', value={'numbers': [1e-6, 1e16, 1.0, -0.0]})
    command('extension.set', table='project', id='P-parity', namespace='example', value={'numbers': [1e-6, 1e16, 1, 0]})
    for args in (['check'], ['query'], ['query', '--view', 'history'], ['query', '--view', 'iteration-progress', '--id', iteration], ['query', '--view', 'show', '--table', 'topics', '--id', topic]):
        a = run(False, args + ['--store', str(old)])
        b = run(True, args + ['--store', str(new)])
        assert a == b, args
    for native, folder in ((False, old), (True, new)):
        before = (folder / 'state.json').read_bytes()
        run(native, ['command', '--store', str(folder), '--request', '-', '--expect-revision', 'stale'], json.dumps(dict(action='task.correct', params={'id':task, 'result':'Wrong'}, date='2026-11-06', reason='Test', evidence='Proof', decision='Decision')), success=False)
        assert before == (folder / 'state.json').read_bytes()
print('20 command sequences, 5 views, stale-write refusal: Python/Go outputs and persisted snapshots match; native PATH is empty.')
