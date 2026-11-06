"""Original migration assertions with native Go conversion checked byte for byte.
The file-writing fault test still uses its Python oracle writer; native rollback
is verified independently in internal/migration tests.
"""
import inspect,json,os,subprocess,sys,unittest
from pathlib import Path
baseline=Path(os.environ['RUM_BASELINE'])
tests=Path(os.environ['RUM_CORE_TESTS'])
sys.path[:0]=[str(baseline/'runtime/scripts'),str(tests)]
import migrate
original=migrate.prepare
native=subprocess.Popen([os.environ['RUM_MIGRATION_CONTRACT']],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True)
count=0
def prepare(*args,**kwargs):
    global count
    p=inspect.signature(original).bind(*args,**kwargs);p.apply_defaults();p=dict(p.arguments)
    native.stdin.write(json.dumps({'Source':str(p['source']),'ProjectID':p['project_id'],'Name':p['name'],'Date':p['date'],'ActiveMilestone':p['active_milestone'] or ''})+'\n');native.stdin.flush()
    result=json.loads(native.stdout.readline());count+=1
    try:state,files,report=original(*args,**kwargs)
    except (ValueError,OSError,KeyError,TypeError) as error:
        assert 'error' in result,('Go accepted invalid migration',error)
        raise
    assert 'error' not in result,result
    assert state==result['state'],('state mismatch',state,result['state'])
    assert report==result['report'],('report mismatch',report,result['report'])
    expected={k:v.hex() for k,v in files.items()}
    assert expected==result['files'],('file byte mismatch',[k for k in set(expected)|set(result['files']) if expected.get(k)!=result['files'].get(k)])
    return result['state'],{k:bytes.fromhex(v) for k,v in result['files'].items()},result['report']
migrate.prepare=prepare
try:
    suite=unittest.defaultTestLoader.discover(str(tests),pattern='test_migration.py')
    result=unittest.TextTestRunner(verbosity=2).run(suite)
finally:
    native.stdin.close();native.wait(timeout=10);native.stdout.close()
    print('NATIVE_MIGRATION_PREPARES='+str(count))
sys.exit(0 if result.wasSuccessful() else 1)
