"""Check baseline preservation, embedded resources, archives and source hashes."""
import hashlib
import json
import os
import re
from pathlib import Path
import tarfile
import zipfile

root=Path(__file__).resolve().parents[1]
baseline=Path(os.environ['RUM_BASELINE'])
digest=lambda raw:hashlib.sha256(raw).hexdigest()
manifest=json.loads((baseline/'MANIFEST.json').read_text())
assert manifest['version']=='0.8.5'
for name,expected in manifest['files'].items():
    assert digest((baseline/name).read_bytes())==expected,('baseline changed',name)
assets={}
for path in (baseline/'resources').rglob('*'):
    if not path.is_file() or '__pycache__' in path.parts:continue
    name=path.relative_to(baseline/'resources').as_posix()
    expected=digest(path.read_bytes())
    assert digest((root/'internal/assets/resources'/name).read_bytes())==expected,('project resource changed',name)
    assets[name]=expected
version=re.search(r'const Version = "([^"]+)"',(root/'internal/release/version.go').read_text()).group(1)
release=root/'build/native'/version
checksums=json.loads((release/'checksums.json').read_text())
assert checksums['version']==version
for name,expected in checksums['files'].items():
    assert digest((release/name).read_bytes())==expected,('release checksum',name)
with zipfile.ZipFile(release/('rumsdlc-'+version+'-source.zip')) as archive:
    prefix='rum-sdlc-device-v'+version+'/'
    source_manifest=json.loads(archive.read(prefix+'MANIFEST.json'))
    assert source_manifest['version']==version and source_manifest['core_version']=='4.1.2'
    for name,expected in source_manifest['files'].items():
        assert digest(archive.read(prefix+name))==expected,('source archive checksum',name)
        assert archive.read(prefix+name)==(root/name).read_bytes(),('stale source archive',name)
targets=[]
for system in ('darwin','linux','windows'):
    for architecture in ('amd64','arm64'):
        target=system+'-'+architecture;targets.append(target)
        binary='rum-'+target+('.exe' if system=='windows' else '')
        if system=='windows':
            with zipfile.ZipFile(release/('rumsdlc-'+version+'-'+target+'.zip')) as archive:
                assert archive.namelist()==['rum.exe']
                assert archive.read('rum.exe')==(release/binary).read_bytes()
        else:
            with tarfile.open(release/('rumsdlc-'+version+'-'+target+'.tar.gz')) as archive:
                assert archive.getnames()==['rum']
                assert archive.getmember('rum').mode==0o755
                assert archive.extractfile('rum').read()==(release/binary).read_bytes()
source={}
for folder in ('cmd','internal','tests','.github','installer-skill'):
    for path in (root/folder).rglob('*'):
        if path.is_file() and '__pycache__' not in path.parts:
            source[path.relative_to(root).as_posix()]=digest(path.read_bytes())
source['go.mod']=digest((root/'go.mod').read_bytes())
report={'baseline_manifest_files_checked':len(manifest['files']),'project_assets_checked':len(assets),'changed':[],'asset_hashes':assets,'release_targets':targets,'release_files_checked':len(checksums['files']),'source_files':source}
(root/'evidence/final-preservation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print('PASS:',len(manifest['files']),'baseline files,',len(assets),'unchanged project assets,',len(targets),'distributions with exact archive contents; source hashes recorded')
