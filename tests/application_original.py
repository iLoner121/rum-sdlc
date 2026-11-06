"""Run original test assertions with Device.call transported to native Go.

FakeHub is a test control-plane fixture, never a product implementation. Calls
into fixture Hub methods stay in Python; native Go owns application operations,
state persistence, Git operations and generated project entry points.
Python installer and other original subprocess paths are NOT replaced here.
Tests that monkeypatch Python implementation internals require native fault
injection counterparts; failures remain visible and are never skipped.
"""
import base64
import threading
from types import SimpleNamespace
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest

baseline = Path(os.environ['RUM_BASELINE'])
sys.path[:0] = [str(baseline), str(baseline/'tests')]
from rumsdlc.device import Device
from rumsdlc.common import Error

from rumsdlc.sync import Sync
from rumsdlc import workspace,project,observation

# Release expectations advance; historical fixture packages remain unchanged.
release_version=subprocess.check_output([os.environ['RUM_APP_CONTRACT'],'--version'],text=True).strip()
import rumsdlc
rumsdlc.VERSION=release_version
workspace.VERSION=release_version
# Python fixture readers still validate project configuration. Extend only their
# historical version allowlist so they can inspect the native release's output.
import types
def extend_versions(code):
    constants=[]
    for value in code.co_consts:
        if isinstance(value,types.CodeType):value=extend_versions(value)
        elif isinstance(value,frozenset) and '0.8.5' in value:value=value|{release_version}
        constants.append(value)
    return code.replace(co_consts=tuple(constants))
project.validate.__code__=extend_versions(project.validate.__code__)

# Preserve original assertions and monkeypatches. Only a test build exposes the
# native continuations below. Production binaries cannot accept fault hooks.
local = threading.local()
points = {
 'sync.git': (Sync,'git'), 'sync.remote_head': (Sync,'remote_head'),
 'device.save_changes': (Device,'save_changes'), 'project.save':(project,'save'),
 'workspace.generated_files':(workspace,'generated_files'),
 'workspace.prepare_agent_update':(workspace,'prepare_agent_update'),
 'workspace.install_agent':(workspace,'install_agent'),
 'workspace.agent_output':(workspace,'agent_output'),
 'workspace.update_configuration':(workspace,'update_configuration'),
 'workspace._write_resource':(workspace,'_write_resource'),
 'observation.read_review':(observation,'read_review'),
}
proxies={}
def decode_result(name,value):
    if name=='workspace.update_configuration':return base64.b64decode(value)
    if name=='sync.git':
        return subprocess.CompletedProcess([],value['Code'],value['Stdout'],value['Stderr'])
    if name=='workspace.generated_files':
        return {k:base64.b64decode(v) for k,v in value.items()}
    return value

def encode_result(name,value):
    if name=='workspace.update_configuration':return base64.b64encode(value).decode()
    if name=='sync.git':
        return {'Code':value.returncode,'Stdout':value.stdout,'Stderr':value.stderr}
    if name=='workspace.generated_files':
        return {k:base64.b64encode(v).decode() for k,v in value.items()}
    return value

def make_proxy(name,original):
    def proxy(*args,**kwargs):
        frames=getattr(local,'frames',[])
        if frames and frames[-1][0]==name:
            return frames[-1][1]()
        return original(*args,**kwargs)
    return proxy
for name,(owner,attribute) in points.items():
    proxy=make_proxy(name,getattr(owner,attribute));proxies[name]=proxy
    setattr(owner,attribute,proxy)

counts = {}
def native_call(self, method, params=None):
    counts[method] = counts.get(method, 0) + 1
    process = subprocess.Popen([os.environ['RUM_APP_CONTRACT']], stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    failures=[]
    def send(value):
        process.stdin.write(json.dumps(value, default=lambda v: str(v) if isinstance(v, Path) else v) + '\n')
        process.stdin.flush()
    enabled=[name for name,(owner,attribute) in points.items() if getattr(owner,attribute) is not proxies[name]]
    send({'home': str(self.home), 'method': method, 'params': params or {},'hooks':enabled})
    def receive():
        while True:
            raw = process.stdout.readline()
            if not raw:
                raise AssertionError('Native process ended without result: ' + process.stderr.read())
            response = json.loads(raw)
            if 'callback' in response:
                hub = self.github
                if response['login'] and hasattr(hub, 'for_login'):
                    hub = hub.for_login(response['login'])
                try:
                    result = getattr(hub, response['callback'])(*(response['args'] or []))
                    send({'result': result})
                except Exception as error:
                    send({'error': str(error)})
                continue
            if 'effect' in response:
                name=response['effect'];args=response['args'];owner,attribute=points[name]
                def proceed():
                    send({'continue':True})
                    result=receive()
                    if 'continuation_error' in result:raise Error(result['continuation_error'])
                    return decode_result(name,result['continuation_result'])
                local.frames=getattr(local,'frames',[])
                local.frames.append((name,proceed))
                try:
                    if name=='sync.git':
                        args=[SimpleNamespace(home=self.home),Path(args[0]),*args[1]]
                        value=getattr(owner,attribute)(*args,check=response['args'][2])
                    elif name=='sync.remote_head':
                        value=getattr(owner,attribute)(SimpleNamespace(home=self.home),Path(args[0]),args[1])
                    elif name=='device.save_changes':value=getattr(owner,attribute)(self,*args)
                    elif name=='project.save':value=getattr(owner,attribute)(Path(args[0]),args[1])
                    elif name=='workspace.generated_files':
                        value=getattr(owner,attribute)(Path(args[0]),Path(args[1]),args[2],Path(args[3]),args[4])
                    elif name=='workspace._write_resource':
                        value=getattr(owner,attribute)(Path(args[0]),bytes.fromhex(args[1]) if args[1] is not None else None)
                    elif name=='workspace.install_agent':
                        value=getattr(owner,attribute)(self,args[0],physical=Path(args[1]) if args[1] else None,update_config=args[2],prepared=args[3])
                    elif name=='workspace.prepare_agent_update':
                        value=getattr(owner,attribute)(self,args[0],physical=Path(args[1]) if args[1] else None,update_config=args[2])
                    elif name=='workspace.agent_output':
                        value=getattr(owner,attribute)(self,args[0],physical=Path(args[1]) if args[1] else None)
                    else:value=getattr(owner,attribute)(*args)
                    send({'result':encode_result(name,value)})
                except Exception as error:
                    failures.append(error);send({'error':str(error)})
                finally:local.frames.pop()
                continue
            return response
    try:
        response=receive()
        if 'error' in response:
            if failures and str(failures[-1]) in response['error']:raise failures[-1]
            raise Error(response['error'])
        return response['result']
    finally:
        process.stdin.close()
        process.wait(timeout=10)
        process.stdout.close()
        process.stderr.close()

Device.call = native_call
if __name__ == '__main__':
    suite = unittest.defaultTestLoader.discover(str(baseline/'tests'),pattern=os.environ.get('RUM_PATTERN','test_device.py'))
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print('NATIVE_APPLICATION_CALLS=' + json.dumps(counts,sort_keys=True))
    sys.exit(0 if result.wasSuccessful() else 1)
