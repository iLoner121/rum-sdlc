"""Project-side transport to the native system; contains no workflow rules."""
import json
import os
from pathlib import Path
import subprocess

def call(operation, **params):
    runtime = os.environ.get('RUMSDLC_NATIVE_RUNTIME')
    home = os.environ.get('RUMSDLC_HOME')
    for parent in Path(__file__).resolve().parents:
        context = parent / '.rum-context.json'
        if context.is_file():
            value = json.loads(context.read_text())
            runtime, home = value['runtime'], value['device_home']
            break
    if not runtime or not home:
        raise ValueError('Native RumSDLC runtime context is missing')
    result = subprocess.run([runtime, '--home', home, 'bridge'],
                            input=json.dumps(dict(operation=operation, **params)),
                            text=True, capture_output=True)
    if result.returncode:
        try:
            message = json.loads(result.stderr)['error']
        except (ValueError, KeyError):
            message = result.stderr.strip() or 'Native RumSDLC operation failed'
        raise ValueError(message)
    return json.loads(result.stdout)['result']
