#!/usr/bin/env python3
"""RumSDLC-only release reconciliation. Never installed in managed projects."""
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile

REPO = os.environ.get('GITHUB_REPOSITORY', 'iLoner121/rum-sdlc')

def run(*args):
    return subprocess.check_output(args, text=True).strip()

def api(endpoint, method='GET', data=None):
    args = ['gh', 'api', endpoint, '--method', method]
    if data is not None:
        args += ['--input', '-']
    p = subprocess.run(args, input=None if data is None else json.dumps(data), text=True, capture_output=True)
    if p.returncode:
        raise RuntimeError(p.stderr.strip())
    return json.loads(p.stdout) if p.stdout.strip() else None

def version_at(sha):
    text = run('git', 'show', f'{sha}:internal/release/version.go')
    version = re.search(r'const Version = "([0-9]+\.[0-9]+\.[0-9]+)"', text).group(1)
    manifest = json.loads(run('git', 'show', f'{sha}:.release-please-manifest.json'))
    if manifest['.'] != version:
        raise RuntimeError('Source version and release manifest differ')
    return version

def key(v):
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+', v):
        raise RuntimeError('Invalid stable version')
    return tuple(map(int, v.split('.')))

def baseline():
    return api(f'repos/{REPO}/releases/latest')['tag_name']

def pending():
    # Reconcile all merged candidates, not just the last webhook event. A queued
    # workflow can be coalesced by GitHub concurrency without losing a release.
    pulls = api(f'repos/{REPO}/pulls?state=closed&sort=updated&direction=desc&per_page=100')
    rows = [p for p in pulls if p['merged_at'] and p['base']['ref']=='main'
            and p['head']['ref'].startswith('release-please--')
            and p['head']['repo'] and p['head']['repo']['full_name']==REPO
            and p['user']['login']=='github-actions[bot]']
    return sorted(rows, key=lambda p:p['merged_at'])

def output(values):
    print(json.dumps(values, indent=2))
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'], 'a') as f:
            for k,v in values.items():
                f.write(f'{k}={str(v).lower() if isinstance(v,bool) else v}\n')

def plan():
    tag=baseline()
    latest=key(tag.removeprefix('v'))
    for p in pending():
        sha=p['merge_commit_sha']
        version=version_at(sha)
        if key(version)>latest:
            expected=json.loads(run('go','run','./cmd/release-plan','--base',tag,'--source',sha))
            if key(version)<key(expected['version']):
                raise RuntimeError('Merged release understates its change impact')
            output({'publish':True,'prepare':False,'sha':sha,'version':version,'pr':p['number']})
            return
    sha=run('git','rev-parse','HEAD')
    p=json.loads(run('go','run','./cmd/release-plan','--base',tag,'--source',sha))
    output({'publish':False,'prepare':p['releasable'],'sha':sha,'version':p['version'],'pr':0})

def publish(sha,version,pr):
    if version_at(sha)!=version or run('git','rev-parse','HEAD')!=sha:
        raise RuntimeError('Publish checkout does not match approved release SHA')
    tag='v'+version
    directory=pathlib.Path('build/native')/version
    sums=json.loads((directory/'checksums.json').read_text())
    if sums.get('source_commit')!=sha or sums.get('channel')!='stable' or sums['version']!=version:
        raise RuntimeError('Distribution identity mismatch')
    required={'checksums.json','release-manifest.json',f'rumsdlc-{version}-source.zip'}
    for system in ('darwin','linux','windows'):
        for arch in ('amd64','arm64'):
            suffix='.zip' if system=='windows' else '.tar.gz'
            required.add(f'rumsdlc-{version}-{system}-{arch}{suffix}')
    files={p.name:p for p in directory.iterdir() if p.is_file()}
    if not required<=files.keys():
        raise RuntimeError('Incomplete six-platform distribution')
    for name,digest in sums['files'].items():
        if name not in files or hashlib.sha256(files[name].read_bytes()).hexdigest()!=digest:
            raise RuntimeError('Distribution checksum mismatch: '+name)
    native=files['rum-linux-amd64']
    identity=json.loads(run(str(native.resolve()),'version'))['result']
    if identity['version']!=version or identity['channel']!='stable' or identity['commit']!=sha or identity['dirty']:
        raise RuntimeError('Built native program identity mismatch')
    if not (required-{'checksums.json'})<=sums['files'].keys():
        raise RuntimeError('Required release files lack checksums')
    refs=run('git','ls-remote','origin',f'refs/tags/{tag}',f'refs/tags/{tag}^{{}}').splitlines()
    if refs:
        refmap={x.split()[1]:x.split()[0] for x in refs}
        target=refmap.get(f'refs/tags/{tag}^{{}}',refmap.get(f'refs/tags/{tag}'))
        if target!=sha:
            raise RuntimeError('Existing immutable tag points to another commit')
    else:
        api(f'repos/{REPO}/git/refs','POST',{'ref':f'refs/tags/{tag}','sha':sha})
    releases=api(f'repos/{REPO}/releases?per_page=100')
    matches=[r for r in releases if r['tag_name']==tag]
    if len(matches)>1:
        raise RuntimeError('Ambiguous release')
    r=matches[0] if matches else api(f'repos/{REPO}/releases','POST',{
        'tag_name':tag,'target_commitish':sha,'name':f'RumSDLC {version}',
        'body':f'Release source: `{sha}`\n\nSee CHANGELOG.md and checksums.json.\n',
        'draft':True,'prerelease':False})
    existing={a['name']:a for a in r['assets']}
    for name in sorted(files):
        path=files[name]
        if name in existing:
            # Verify actual bytes, including releases created before digest API.
            p=subprocess.run(['gh','api',f'repos/{REPO}/releases/assets/{existing[name]["id"]}',
                '-H','Accept: application/octet-stream'],capture_output=True)
            if p.returncode or hashlib.sha256(p.stdout).digest()!=hashlib.sha256(path.read_bytes()).digest():
                raise RuntimeError('Existing asset differs; never overwrite: '+name)
        else:
            if not r['draft']:
                raise RuntimeError('Published release is missing an asset; create a new version')
            subprocess.run(['gh','release','upload',tag,str(path),'--repo',REPO],check=True)
    fresh=api(f'repos/{REPO}/releases/{r["id"]}')
    if not required<={a['name'] for a in fresh['assets']}:
        raise RuntimeError('Release assets incomplete after upload')
    if fresh['draft']:
        api(f'repos/{REPO}/releases/{r["id"]}','PATCH',{'draft':False,'prerelease':False,'make_latest':'true'})
    api(f'repos/{REPO}/issues/{pr}/labels','POST',{'labels':['autorelease: tagged']})
    # The PR is already merged; this is release bookkeeping only.
    label='autorelease%3A%20pending'
    subprocess.run(['gh','api',f'repos/{REPO}/issues/{pr}/labels/{label}','--method','DELETE'],capture_output=True)
    print(f'Published {tag} at {sha}')

if __name__=='__main__':
    if sys.argv[1]=='plan':plan()
    elif sys.argv[1]=='publish':publish(*sys.argv[2:5])
    else:raise SystemExit('Unknown release operation')
