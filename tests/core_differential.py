"""Run unchanged historical tests with every in-process transition checked in Go.
Python remains the test oracle only; the tested executable contains native Go.
"""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest
import threading

BASE = Path(os.environ['RUM_BASELINE'])
TESTS = Path(os.environ['RUM_TESTS'])
sys.path.insert(0, str(TESTS))
sys.path.insert(0, str(BASE / 'runtime/scripts'))
import service
import model
import queries
import inspect
import validation
import store

process = subprocess.Popen([os.environ['RUM_CORE_BINARY']], stdin=subprocess.PIPE,
                           stdout=subprocess.PIPE, text=True)
counters = {'dispatch': 0, 'validate': 0, 'actions': {}}
original_dispatch, original_validate = service.dispatch, model.validate
original_query = queries.query
counters['query'] = 0
counters['legacy_fixture_init'] = 0
call_lock = threading.Lock()

def call(payload):
    with call_lock:
        process.stdin.write(json.dumps(payload, ensure_ascii=False) + '\n')
        process.stdin.flush()
        line = process.stdout.readline()
    if not line:
        raise AssertionError('Go contract process terminated')
    response = json.loads(line)
    if response.get('panic'):
        raise AssertionError('Go panicked: ' + response['error'])
    return response

def dispatch(state, request):
    if request.get("action") == "project.init" and validation.VERSION != "4.1.2":
        # The historical test patches the old Python version to construct a legacy fixture.
        # Only this fixture construction stays in Python; its validation and every
        # subsequent operation are checked against the native executable.
        after, identifier = original_dispatch(state, request)
        validate(after)
        counters["legacy_fixture_init"] += 1
        return after, identifier
    before = copy.deepcopy(state)
    result = call({'operation': 'dispatch', 'state': before, 'request': request})
    counters['dispatch'] += 1
    action = request.get('action')
    counters['actions'][action] = counters['actions'].get(action, 0) + 1
    try:
        after, identifier = original_dispatch(state, request)
        original_validate(after)
    except (ValueError, KeyError, TypeError) as error:
        if 'error' not in result:
            raise AssertionError(f'Go accepted Python rejection: {request}: {error}')
        raise
    if 'error' in result:
        raise AssertionError(f'Go rejected Python success: {request}: {result}')
    assert after == result['state'], f'State mismatch: {request}'
    assert identifier == result['result'], f'Result mismatch: {request}'
    expected = store.digest({'before': before, 'request': request, 'after': after})
    assert expected == result['revision'], f'Token mismatch: {request}'
    # Use the Go output for subsequent assertions and persisted snapshots.
    return result['state'], result['result']

def validate(state):
    result = call({'operation': 'validate', 'state': state})
    counters['validate'] += 1
    try:
        original_validate(state)
    except (ValueError, KeyError, TypeError):
        if 'error' not in result:
            raise AssertionError('Go validator accepted invalid Python snapshot')
        raise
    if 'error' in result:
        raise AssertionError('Go validator rejected valid snapshot: ' + result['error'])

def query(state, *args, **kwargs):
    bound = inspect.signature(original_query).bind(state, *args, **kwargs)
    bound.apply_defaults()
    params = dict(bound.arguments)
    params.pop('state')
    response = call({'operation': 'query', 'state': state, 'params': params})
    counters['query'] += 1
    try:
        expected = original_query(state, *args, **kwargs)
    except (ValueError, KeyError, TypeError):
        if 'error' not in response:
            raise AssertionError('Go accepted invalid query: ' + repr(params))
        raise
    if 'error' in response:
        raise AssertionError('Go rejected valid query: ' + repr(params) + ': ' + response['error'])
    assert expected == response['result'], f'Query mismatch: {params}: {expected} != {response["result"]}'
    return response['result']

service.dispatch, model.validate, queries.query = dispatch, validate, query
def close():
    process.stdin.close()
    process.wait(timeout=10)
    process.stdout.close()
    print('GO_DIFFERENTIAL_COUNTS=' + json.dumps(counters, sort_keys=True))
if __name__ == '__main__':
    try:
        suite = unittest.defaultTestLoader.discover(str(TESTS), pattern=os.environ.get('RUM_PATTERN', 'test_*.py'))
        result = unittest.TextTestRunner(verbosity=2).run(suite)
    finally:close()
    sys.exit(0 if result.wasSuccessful() else 1)
