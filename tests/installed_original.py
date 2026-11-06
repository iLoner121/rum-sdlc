"""Run original installed workflows against the native installer/CLI/service.
Only executable invocation is adapted; original assertions stay in their modules.
Historical Python releases remain fixtures for explicit upgrade scenarios.
"""
import json,os,subprocess,sys,unittest
from pathlib import Path
import application_original as transport
core_transport=None
if os.environ.get('RUM_CORE_BINARY'):
    os.environ.setdefault('RUM_TESTS',str(transport.baseline/'tests'))
    import core_differential as core_transport
import service_original
import test_v083
from test_install_cli import InstallCliTests
binary=Path(os.environ['RUM_NATIVE_BINARY']).resolve()
original_setup=InstallCliTests.setUp

def setup(self):
    original_setup(self)
    self.env['RUM_TEST_OFFLINE_HUB']='1'
    self.env['GH_TOKEN']='fake-fixture-token'
    self.env['GITHUB_TOKEN']='fake-fixture-token'

def install(self,*args,check=True):
    return subprocess.run([str(binary),'--home',str(self.home),'install','--destination',str(self.destination),'--bin-dir',str(self.root/'commands'),*args],text=True,capture_output=True,env=self.env,check=check,timeout=30)

def cli(self,*args,check=True):
    argv=[str(self.destination/'rum'),'--home',str(self.home),*map(str,args)]
    try:return subprocess.run(argv,text=True,capture_output=True,env=self.env,check=check,timeout=30)
    except FileNotFoundError as error:
        if check:raise
        return subprocess.CompletedProcess(argv,127,'',str(error))

InstallCliTests.setUp=setup
InstallCliTests.install=install
InstallCliTests.cli=cli
# The immutable release entry is a native executable named rum, not rum.py.
# Keep the original corruption assertion while targeting the actual native entry.
case=InstallCliTests.test_installer_rejects_changed_existing_release
case.__code__=case.__code__.replace(co_consts=tuple('rum' if value=='rum.py' else value for value in case.__code__.co_consts))
# Pinned Python system modules were compiled into rum. Their native dependency
# counterparts are that executable, its manifest, and the small project bridge.
from test_v081 import ReadinessTests

def pinned_dependencies(self):
    rec,root=self.fresh()
    context=json.loads((root/'.rum-context.json').read_text())
    paths=[self.destination/'rum',self.destination/'MANIFEST.json',*sorted(Path(context['core_scripts']).glob('*.py'))]
    for path in paths:
        with self.subTest(resource=str(path)):
            data=path.read_bytes();mode=path.stat().st_mode;path.unlink()
            try:self.not_healthy(rec)
            finally:path.write_bytes(data);path.chmod(mode)
    self.assertTrue(self.health(rec)['ok'])

def pinned_corruption(self):
    rec,root=self.fresh()
    context=json.loads((root/'.rum-context.json').read_text())
    path=Path(context['core_scripts'])/'git_validation.py'
    data=path.read_bytes();path.write_bytes(data+b'\n# changed\n')
    self.not_healthy(rec);path.write_bytes(data);self.assertTrue(self.health(rec)['ok'])

ReadinessTests.test_each_pinned_core_dependency_missing_never_reports_healthy=pinned_dependencies
ReadinessTests.test_core_corruption_is_not_treated_as_custom_guidance=pinned_corruption
from test_v082 import EntryAndSnapshotTests
original_fresh=EntryAndSnapshotTests.fresh
def fresh(self):
    result=original_fresh(self)
    if self._testMethodName=='test_editor_after_individual_write_keeps_recovery_evidence':
        # Native installs use a stable launcher across upgrades. Seed an older
        # managed context so this original post-write race actually has a write,
        # as the Python source-versus-release runtime paths did in the baseline.
        import hashlib
        root=Path(result[0]['workspace']);path=root/'.rum-context.json'
        context=json.loads(path.read_text());context['runtime']=str(self.destination/'rum')
        path.write_text(json.dumps(context)+'\n')
        inventory=root/'.rum-agent-files.json';value=json.loads(inventory.read_text())
        value['.rum-context.json']=hashlib.sha256(path.read_bytes()).hexdigest()
        inventory.write_text(json.dumps(value)+'\n')
    return result
EntryAndSnapshotTests.fresh=fresh
# Retain original injection code and assertions, transporting the two native
# activation side effects. Historical installers remain historical fixtures.
from test_v080 import OfflineActivationTests
original_fault=OfflineActivationTests.fault
def fault(self,hook,source=transport.baseline):
    if Path(source)!=transport.baseline:return original_fault(self,hook,source)
    spec={'hook':hook,'home':str(self.home),'destination':str(self.destination),'bin_dir':str(self.root/'commands')}
    return subprocess.run([sys.executable,'-B',str(Path(__file__).with_name('installation_fault.py')),json.dumps(spec)],env=self.env,capture_output=True,text=True,timeout=45)
OfflineActivationTests.fault=fault
case=OfflineActivationTests.test_committed_cleanup_failure_does_not_revert_upgrade
case.__code__=case.__code__.replace(co_consts=tuple('rum' if value=='rum.py' else value for value in case.__code__.co_consts))
# Redirect only subprocess invocations of the removed installed system rum.py.
# Generated project rum.py and historical fixture releases remain Python scripts.
original_run=subprocess.run

def run_entry(argv,*args,**kwargs):
    if isinstance(argv,(list,tuple)) and len(argv)>2 and str(argv[0])==sys.executable:
        index=2 if argv[1]=='-B' else 1
        entry=Path(str(argv[index]))
        if entry==transport.baseline/'install.py':
            # The native installer is a rum subcommand, with global home first.
            operands=list(argv[index+1:]);home_index=operands.index('--home')
            home=operands[home_index+1];del operands[home_index:home_index+2]
            argv=[str(binary),'--home',home,'install',*operands]
        elif entry==transport.baseline/'rum.py':
            argv=[str(binary),*argv[index+1:]]
        elif str(argv[index])=='-c' and len(argv)>index+1 and 'from rumsdlc.device import Device;Device(' in str(argv[index+1]):
            argv=list(argv)
            argv[index+1]='import sys;sys.path.insert(0,'+repr(str(Path(__file__).parent))+');import application_original;'+str(argv[index+1])
        if entry.name=='project-fault.py':
            # Test-generated driver imports the same native Device transport
            # before applying its original resource-write monkeypatch.
            content=entry.read_text()
            bootstrap='import sys;sys.path.insert(0,'+repr(str(Path(__file__).parent))+');import application_original\n'
            if not content.startswith(bootstrap):entry.write_text(bootstrap+content)
        if entry.name=='rum.py' and not entry.exists() and (entry.parent/'rum').is_file():
            argv=[str(entry.parent/'rum'),*argv[index+1:]]
    return original_run(argv,*args,**kwargs)
subprocess.run=run_entry
original_popen=subprocess.Popen
def popen_entry(argv,*args,**kwargs):
    if isinstance(argv,(list,tuple)) and len(argv)==8 and argv[0]==sys.executable and list(argv[1:3])==['-B','-c'] and 'serve(Path(sys.argv[4])' in argv[3]:
        # Same immutable FakeHub JSON, actual native server in the caller-supplied cwd.
        argv=[os.environ['RUM_APP_CONTRACT'],'--serve-fixture',str(argv[7]),str(argv[6])]
    return original_popen(argv,*args,**kwargs)
subprocess.Popen=popen_entry
def capture_native_request(args):
    import rumsdlc.cli as original_cli
    result=subprocess.run([os.environ['RUM_APP_CONTRACT'],'--prepare',*args],text=True,capture_output=True,check=True)
    request=json.loads(result.stdout)['result']
    return original_cli.request(Path(request['home']),request['method'],request['params'])
test_v083.main=capture_native_request

suite=unittest.defaultTestLoader.discover(str(transport.baseline/'tests'),pattern=os.environ.get('RUM_PATTERN','test_e2e_local.py'))
# The 0.8.5 suite's explicit "current version" assertions now expect 0.9.0.
# Change only exact version literals in test/helper code, never historical paths,
# original fixture releases, business assertions, or returned application data.
import types
def current_version_literals(code):
    return code.replace(co_consts=tuple(current_version_literals(v) if isinstance(v,types.CodeType) else transport.release_version if v=='0.8.5' else v for v in code.co_consts))
seen=set()
for module in list(sys.modules.values()):
    if not getattr(module,'__name__','').startswith('test_'):continue
    for cls in list(vars(module).values()):
        if not isinstance(cls,type) or not issubclass(cls,unittest.TestCase):continue
        for function in vars(cls).values():
            if isinstance(function,types.FunctionType) and id(function) not in seen:
                seen.add(id(function));function.__code__=current_version_literals(function.__code__)
result=unittest.TextTestRunner(verbosity=2).run(suite)
if core_transport:core_transport.close()
print('NATIVE_APPLICATION_CALLS='+json.dumps(transport.counts,sort_keys=True))
sys.exit(0 if result.wasSuccessful() else 1)
