"""Execute original activation fault hooks at native installer boundaries.

Only the contracttest binary accepts these callbacks. Abrupt termination kills
the native writer before closing the protocol, preserving its recovery journal.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
from types import SimpleNamespace

spec = json.loads(sys.argv[1])
home = Path(spec['home'])
process = subprocess.Popen([os.environ['RUM_APP_CONTRACT']], stdin=subprocess.PIPE,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

def send(value):
    process.stdin.write(json.dumps(value) + '\n')
    process.stdin.flush()

def receive():
    raw = process.stdout.readline()
    if not raw:
        raise RuntimeError('Native installer ended without response: ' + process.stderr.read())
    return json.loads(raw)

def original(*args):
    send({'continue': True})
    reply = receive()
    if 'continuation_error' in reply:
        raise OSError(reply['continuation_error'])
    return reply['continuation_result']

real_exit = os._exit
def crash(code):
    process.kill()
    process.wait(timeout=10)
    real_exit(code)

os._exit = crash
tx = SimpleNamespace(replace=original, clear=original)
exec(spec['hook'])
send({'home': str(home), 'method': 'installation',
      'params': {'destination': spec['destination'], 'bin_dir': spec['bin_dir']},
      'hooks': ['installation.replace', 'installation.clear']})
try:
    while True:
        reply = receive()
        if 'effect' not in reply:
            if 'error' in reply:
                raise RuntimeError(reply['error'])
            print(json.dumps(reply['result']))
            break
        args = reply['args']
        args[0] = Path(args[0])
        try:
            value = getattr(tx, reply['effect'].split('.')[1])(*args)
            send({'result': value})
        except Exception as error:
            send({'error': str(error)})
finally:
    process.stdin.close()
    process.wait(timeout=10)
    process.stdout.close()
    process.stderr.close()
