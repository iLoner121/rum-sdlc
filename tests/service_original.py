"""Native service adapters for original RPC fixtures and cancellation assertions."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import test_device
import test_v04
from rumsdlc.daemon import request

def start(home, hub, slow=False):
    descriptor, name = tempfile.mkstemp(prefix='rum-hub-',suffix='.json')
    with os.fdopen(descriptor,'w') as output:json.dump(hub.repos,output)
    argv=[os.environ['RUM_APP_CONTRACT'],'--serve-fixture',str(home),name]
    if slow:argv.append('slow')
    process=subprocess.Popen(argv,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    return process,Path(name)

def serve(home, interval, discovery, device):
    process,fixture=start(home,device.github)
    try:
        output,error=process.communicate(timeout=60)
        if process.returncode:raise RuntimeError(error)
    finally:
        if process.poll() is None:process.kill();process.wait()
        fixture.unlink()

test_device.serve=serve

def cancellation(self):
    # The original Python sleep monkeypatch becomes a real cancellable native
    # child at remote_head. Preserve concurrency, latency, stop and data checks.
    self.enable()
    process,fixture=start(self.a.home,self.hub,slow=True)
    try:
        import selectors
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout,selectors.EVENT_READ)
            self.assertTrue(selector.select(5),'native worker did not enter remote operation')
        self.assertTrue(json.loads(process.stdout.readline())['entered'])
        begin=time.monotonic()
        self.assertEqual(1,request(self.a.home,'list',{})['counts']['local'])
        request(self.a.home,'create',{'path':str(self.base/'other-local'),'name':'Other','date':'2026-11-04'})
        self.assertLess(time.monotonic()-begin,2)
        request(self.a.home,'shutdown',{})
        output,error=process.communicate(timeout=5)
        self.assertEqual(0,process.returncode,error)
    finally:
        if process.poll() is None:process.kill();process.wait()
        fixture.unlink()
        process.stdout.close();process.stderr.close()
    self.assertEqual(2,self.a.call('list')['counts']['local'])

test_v04.ReviewRegressionTests.test_background_network_does_not_block_reads_or_other_projects_and_can_stop=cancellation
