#!/usr/bin/env python3
"""Read-only checks for owner-authored commits: effective identities and message syntax."""
import sys
sys.dont_write_bytecode = True
import agent_bootstrap
import repo_check
import git_validation as gv
import argparse
import json
from pathlib import Path
import re
import subprocess
import validation as v

TYPES = 'feat|fix|refactor|perf|test|docs|build|ci|chore|revert'
TITLE = re.compile(r'(' + TYPES + r')(?:\(([a-z][a-z0-9]*(?:-[a-z0-9]+)*)\))?(!)?: ([\x20-\x7e]+)')
TOOL_NAMES = {'ai', 'assistant', 'agent', 'bot', 'codex', 'chatgpt', 'claude', 'cursor', 'cursor agent', 'copilot', 'github copilot'}
ATTRIBUTION = re.compile(r'^(?:[-*] )?(co[ -]?authored[ -]?by|generated[ -]?by|assisted[ -]?by|author|committer|signed-off-by)\s*:\s*(.*)$', re.I)
ASSISTANCE = re.compile(r'^(?:(?:generated|assisted|authored|written|created|reviewed|powered|made|developed) (?:by|with)\s*:?[ \t]*)(.+)$', re.I)


def git(root, *args):
    return repo_check.git(root, *args).stdout


def identity(name, email):
    v.text(name, 'confirmed name'); v.text(email, 'confirmed email')
    v.need(name == name.strip() and not re.search(r'[\x00-\x1f\x7f<>]', name), 'Invalid confirmed name')
    v.match(email, r'[^\s<>@\x00-\x1f\x7f]+@[^\s<>@\x00-\x1f\x7f]+', 'confirmed email')
    return {'name': name, 'email': email}


def parse_identity(raw):
    match = re.fullmatch(r'(.+) <([^<>]+)> -?[0-9]+ [+-][0-9]{4}', raw.strip())
    v.need(match is not None, 'Invalid Git identity header')
    return {'name': match[1], 'email': match[2]}


def check_message(raw, owner, topic):
    v.need(isinstance(raw, bytes), 'Read message as bytes')
    text = raw.decode('utf-8')
    v.need(text.endswith('\n') and text.strip() and '\r' not in text and '\0' not in text,
           'Message must be nonempty UTF-8, LF-only, with final newline')
    lines = v.unfenced(text).splitlines()
    first = lines[0]
    title = TITLE.fullmatch(first)
    v.need(title is not None and len(first) <= 72, 'Title requires supported type, kebab-case scope, ASCII English summary and at most 72 characters')
    kind, scope, breaking, summary = title.groups()
    v.need(summary == summary.strip() and not summary.endswith('.'), 'Summary must have no surrounding spaces or final period')
    v.need(summary.casefold() not in {'update', 'fix bugs', 'complete task', 'address feedback'}, 'Use a concrete summary')
    v.need(scope != 'wip' or kind == 'chore', 'WIP uses chore(wip)')
    v.need(len(lines) == 1 or not lines[1].strip(), 'Separate title and body/footer by a blank line')
    changes = [line for line in lines if re.match(r'^BREAKING(?: CHANGE|-CHANGE):', line, re.I)]
    v.need((bool(breaking) and len(changes) == 1 and changes[0].startswith('BREAKING CHANGE: ') and changes[0][17:].strip())
           or (not breaking and not changes), '! and one nonempty BREAKING CHANGE footer must occur together')
    expected = owner['name'] + ' <' + owner['email'] + '>'
    for line in lines:
        field = ATTRIBUTION.fullmatch(line)
        if field:
            v.need(field[1].lower() == 'signed-off-by' and field[2] == expected, 'Unauthorized attribution field')
        claim = ASSISTANCE.fullmatch(line)
        if claim:
            value = claim[1].strip().rstrip('.').casefold()
            v.need(value not in TOOL_NAMES, 'AI assistance signature is forbidden')
    return {'type': kind, 'scope': scope, 'topic': topic}


def commit_data(root, ref):
    sha = git(root, 'rev-parse', '--verify', '--end-of-options', ref + '^{commit}').decode().strip()
    raw = git(root, 'cat-file', 'commit', sha)
    headers, message = raw.split(b'\n\n', 1)
    result = {'sha': sha, 'parents': [], 'message': message}
    for line in headers.splitlines():
        for key in ('author', 'committer'):
            if line.startswith(key.encode() + b' '): result[key] = parse_identity(line[len(key)+1:].decode())
        if line.startswith(b'tree '): result['tree'] = line[5:].decode()
        if line.startswith(b'parent '): result['parents'].append(line[7:].decode())
    return result


def run(root, args):
    check = repo_check.inspect(root)
    v.need(check['checks_complete'], 'Incomplete repository checks; follow git-fallback.md')
    owner = identity(args.name, args.email)
    message = Path(args.message).read_bytes()
    convention = check_message(message, owner, args.topic)
    if args.command == 'preflight':
        actual = {key: parse_identity(git(root, 'var', 'GIT_' + key.upper() + '_IDENT').decode()) for key in ('author', 'committer')}
        v.need(all(value == owner for value in actual.values()), 'Effective author/committer differ from confirmed owner; check process environment')
        return dict(ok=True, checks=['effective-identities', 'message-syntax'], identity=owner, **convention)
    actual = commit_data(root, args.commit)
    v.need(actual.get('author') == actual.get('committer') == owner, 'Actual author/committer differ from confirmed owner')
    v.need(actual['message'] == message, 'Actual message differs from the reviewed file')
    return dict(ok=True, checks=['actual-identities', 'exact-message'], identity=owner,
                sha=actual['sha'], tree=actual['tree'], parents=actual['parents'], **convention)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    for command in ('preflight', 'postcheck'):
        child = sub.add_parser(command)
        child.add_argument('--repo', required=True); child.add_argument('--message', required=True)
        child.add_argument('--name', required=True); child.add_argument('--email', required=True)
        child.add_argument('--topic', help='Optional management context only; never added to the commit')
        if command == 'postcheck': child.add_argument('--commit', required=True)
    args = parser.parse_args()
    try:
        root = Path(git(Path(args.repo).expanduser(), 'rev-parse', '--show-toplevel').decode().strip())
        result = run(root, args)
    except (ValueError, OSError, KeyError, UnicodeError) as error:
        print(json.dumps({'ok': False, 'error': str(error)}, ensure_ascii=False)); return 1
    result['limits'] = 'No approval, semantic acceptance, content-tree comparison or history audit is performed'
    print(json.dumps(result, ensure_ascii=False, indent=2)); return 0


if __name__ == '__main__':
    sys.exit(main())
