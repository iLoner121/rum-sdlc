#!/usr/bin/env python3
"""Agent-side bounded repository checks. Never commits, fetches, pushes or grants permission."""
import agent_bootstrap
import argparse
import json
import os
from pathlib import Path
import subprocess
import signal


def git(root, *args, input=None, check=True, timeout=15, readonly=True):
    env = dict(os.environ)
    for key in ('GIT_DIR','GIT_WORK_TREE','GIT_INDEX_FILE','GIT_COMMON_DIR','GIT_OBJECT_DIRECTORY','GIT_ALTERNATE_OBJECT_DIRECTORIES'):
        env.pop(key, None)
    env.update(GIT_TERMINAL_PROMPT='0', GCM_INTERACTIVE='Never')
    prefix = ['git']
    if readonly:
        env.update(GIT_OPTIONAL_LOCKS='0', GIT_NO_LAZY_FETCH='1')
        prefix += ['--no-optional-locks', '-c', 'core.fsmonitor=false']
    command = [*prefix, '-C', str(root), *args]
    with subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, env=env, start_new_session=True) as process:
        try:
            stdout, stderr = process.communicate(input=input, timeout=timeout)
        except subprocess.TimeoutExpired:
            try: os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError: pass
            process.communicate()
            raise ValueError('Git operation timed out; preserve possible partial results and inspect before retry')
        result = subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
    if check and result.returncode:
        raise ValueError('Git operation failed: ' + args[0] + '; inspect repository/environment and any partial result')
    return result


def text(root, *args):
    return git(root, *args).stdout.decode('utf-8').strip()


def inspect(root):
    root = Path(root).expanduser().resolve()
    actual = Path(text(root, 'rev-parse', '--show-toplevel')).resolve()
    if actual != root:
        raise ValueError('Select the exact repository/worktree root')
    issues = []
    stage = git(root, 'ls-files', '--stage', '-z').stdout
    submodules = [p.split(b'\t', 1)[1].decode('utf-8', 'surrogateescape') for p in stage.split(b'\0') if p.startswith(b'160000 ')]
    if submodules: issues.append('submodules-require-recursive-review')
    if text(root, 'rev-parse', '--is-shallow-repository') == 'true': issues.append('shallow-history')
    if git(root, 'config', '--get-regexp', r'^(extensions\.partialclone|remote\..*\.promisor)$', check=False).returncode == 0: issues.append('partial-clone')
    if git(root, 'config', '--bool', '--get', 'core.sparseCheckout', check=False).stdout.strip() == b'true': issues.append('sparse-checkout')
    flags = git(root, 'ls-files', '-v', '-z').stdout.split(b'\0')
    if any(p and (p[:1] == b'S' or p[:1].islower()) for p in flags): issues.append('hidden-index-flags')
    if text(root, 'for-each-ref', '--format=%(refname)', 'refs/replace'): issues.append('replacement-history')
    for name in ('MERGE_HEAD', 'CHERRY_PICK_HEAD', 'REVERT_HEAD', 'rebase-merge', 'rebase-apply', 'info/grafts'):
        p = Path(text(root, 'rev-parse', '--git-path', name))
        if (p if p.is_absolute() else root / p).exists(): issues.append('unfinished-or-modified-history:' + name)
    if git(root, 'ls-files', '-u').stdout: issues.append('unresolved-index')
    if issues:
        # Do not probe incomplete object stores or nested Git environments further.
        return {'ok': False, 'checks_complete': False, 'path': str(root), 'dirty': None,
                'visible_changes': None, 'status_entries': [], 'submodules': submodules,
                'unsupported_or_incomplete': issues, 'push_authorized': False,
                'limits': 'Structural coverage incomplete; content/status checks not run. Follow git-fallback.md.'}
    tracked = git(root, 'ls-files', '-z').stdout
    attrs = git(root, 'check-attr', '-z', '--stdin', 'filter', input=tracked).stdout.split(b'\0')
    if any(attrs[i] not in (b'unspecified', b'unset', b'') for i in range(2, len(attrs), 3)): issues.append('content-filters-require-tool-specific-review')
    configured = git(root, 'config', '--name-only', '--get-regexp', r'^filter\..*\.(clean|smudge|process|required)$', check=False)
    if configured.returncode not in (0, 1): raise ValueError('Cannot inspect filters')
    args = []
    for name in sorted({s.rsplit('.', 1)[0] for s in configured.stdout.decode().splitlines()}):
        for key, value in [('clean',''),('smudge',''),('process',''),('required','false')]: args += ['-c', name+'.'+key+'='+value]
    # Do not launch uninspected submodule filters; coverage explicitly remains incomplete.
    status = git(root, *args, 'status', '--porcelain=v1', '-z', '--untracked-files=all', '--ignored=matching', '--ignore-submodules=all').stdout
    return {'ok': not issues, 'checks_complete': not issues, 'path': str(root),
            'dirty': bool(status) if not issues else None, 'visible_changes': bool(status),
            'status_entries': status.decode('utf-8', 'surrogateescape').split('\0')[:-1],
            'submodules': submodules, 'unsupported_or_incomplete': issues,
            'push_authorized': False,
            'limits': 'Snapshot, not permission. Ignored files are included for preservation. Incomplete checks require the rules fallback.'}


def push_plan(root, remote, target, source, expected_remote):
    if not remote or remote.startswith('-') or any(c.isspace() or ord(c) < 32 for c in remote):
        raise ValueError('Use an explicit valid remote name')
    report = inspect(root)
    if not report['checks_complete']:
        raise ValueError('Incomplete repository checks; use git-fallback.md, do not push')
    if not target.startswith('refs/heads/'):
        raise ValueError('Push target must be a full branch ref')
    git(root, 'check-ref-format', target)
    fetch = text(root, 'remote', 'get-url', '--all', remote).splitlines()
    push = text(root, 'remote', 'get-url', '--push', '--all', remote).splitlines()
    if len(fetch) != 1 or fetch != push:
        raise ValueError('Multiple or different push URLs require explicit provider review')
    if git(root, 'config', '--bool', '--get', 'remote.'+remote+'.mirror', check=False).stdout.strip() == b'true':
        raise ValueError('Mirror remotes are not supported')
    source_sha = text(root, 'rev-parse', '--verify', '--end-of-options', source+'^{commit}')
    advertised = text(root, 'ls-remote', '--heads', remote, target).splitlines()
    if len(advertised) > 1: raise ValueError('Ambiguous remote target')
    remote_sha = advertised[0].split()[0] if advertised else None
    expected = None if expected_remote == 'absent' else expected_remote
    if remote_sha != expected: raise ValueError('Remote target changed; refresh review')
    if remote_sha is None:
        raise ValueError('New remote branch requires reviewed baseline/history; follow git-fallback.md')
    git(root, 'cat-file', '-e', remote_sha+'^{commit}')
    git(root, 'merge-base', '--is-ancestor', remote_sha, source_sha)
    commits = text(root, 'rev-list', '--reverse', remote_sha+'..'+source_sha).splitlines()
    return {'ok': True, 'remote': remote, 'target': target, 'source_sha': source_sha,
            'expected_remote_sha': remote_sha, 'commits_to_review': commits, 'worktree': report,
            'push_authorized': False, 'limits': 'No push performed. Review every outgoing commit, exact tree, tests and authorization; recheck remote before pushing this exact SHA.'}


def main():
    p=argparse.ArgumentParser(description=__doc__);sub=p.add_subparsers(dest='command',required=True)
    for name in ('inspect','push-plan'):
        c=sub.add_parser(name);c.add_argument('--repo',required=True)
        if name=='push-plan':
            for field in ('remote','target','source','expected-remote'):c.add_argument('--'+field,required=True)
    a=p.parse_args()
    try:
        result=inspect(a.repo) if a.command=='inspect' else push_plan(a.repo,a.remote,a.target,a.source,a.expected_remote)
    except (ValueError,OSError,KeyError,TypeError,UnicodeError) as e:
        print(json.dumps({'ok':False,'checks_complete':False,'push_authorized':False,'error':str(e)}));return 1
    print(json.dumps(result,ensure_ascii=True,indent=2));return 0 if result['ok'] else 1

if __name__=='__main__':raise SystemExit(main())
