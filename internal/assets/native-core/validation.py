"""Pure common validation; no storage or Git access."""
import sys
sys.dont_write_bytecode = True
import datetime
import json
import re

VERSION = "4.1.2"
COMPATIBLE_VERSIONS = {"4.1.0", "4.1.1", VERSION}
TOPIC = r"T-[0-9]{8}-[0-9a-f]{8}"
SLUG = r"[a-z0-9]+(?:-[a-z0-9]+)*"


def need(condition, message):
    if not condition:
        raise ValueError(message)


def obj(value, label):
    need(isinstance(value, dict), label + ' must be a JSON object')
    return value


def fields(data, names, label):
    obj(data, label)
    missing = set(names.split()) - set(data)
    need(not missing, label + ' missing fields: ' + ', '.join(sorted(missing)))


def text(value, label):
    need(isinstance(value, str) and bool(value.strip()), label + ' must be nonempty text')
    return value


def match(value, pattern, label):
    need(isinstance(value, str) and re.fullmatch(pattern, value) is not None, 'Invalid ' + label)
    return value


def enum(value, choices, label):
    need(isinstance(value, str) and value in choices, 'Invalid ' + label)
    return value


def date(value):
    match(value, r'[0-9]{4}-[0-9]{2}-[0-9]{2}', 'date; use YYYY-MM-DD in the user timezone')
    try:
        return datetime.date.fromisoformat(value)
    except ValueError:
        raise ValueError('Invalid calendar date: ' + value)


def slug(value):
    match(value, SLUG, 'slug')
    need(len(value) <= 64, 'Slug exceeds 64 characters')
    return value


def ids(value, pattern=TOPIC, label='Topic IDs'):
    need(isinstance(value, list), label + ' must be an array')
    for item in value:
        match(item, pattern, label)
    need(len(value) == len(set(value)), 'Duplicate ' + label)
    return value


def loads(raw, label='JSON'):
    def pairs(items):
        value = {}
        for key, item in items:
            need(key not in value, 'Duplicate JSON key: ' + key)
            value[key] = item
        return value
    def constant(value):
        raise ValueError('Non-finite JSON number: ' + value)
    try:
        return json.loads(raw, object_pairs_hook=pairs, parse_constant=constant)
    except ValueError as error:
        raise ValueError(label + ': ' + str(error)) from error


def unfenced(content):
    """Ignore Markdown fenced examples when reading authoritative Task metadata."""
    result, fence, length = [], None, 0
    for line in content.splitlines():
        opening = re.match(r'^ {0,3}(`{3,}|~{3,})(.*)$', line)
        if fence is None and opening:
            fence, length = opening[1][0], len(opening[1])
            result.append(''); continue
        if fence is not None:
            if re.fullmatch(r' {0,3}' + re.escape(fence) + '{' + str(length) + r',}\s*', line):
                fence = None
            result.append(''); continue
        result.append(line)
    need(fence is None, 'Unclosed Markdown fence in Task')
    return '\n'.join(result)


def cycles(edges, label):
    done, visiting = set(), set()
    for root in edges:
        stack = [(root, False)]
        while stack:
            node, leaving = stack.pop()
            if leaving:
                visiting.remove(node); done.add(node)
                continue
            need(node not in visiting, label + ' cycle at ' + node)
            if node in done:
                continue
            visiting.add(node)
            stack.append((node, True))
            stack.extend((other, False) for other in edges.get(node, []))


def extensions(value):
    obj(value,'External extensions')
    for name,payload in value.items():
        match(name,r'[a-z][a-z0-9]*(?:-[a-z0-9]+)*','Extension namespace')
        obj(payload,'Extension payload')
    try:json.dumps(value,allow_nan=False)
    except (ValueError,TypeError):raise ValueError('Extensions must be finite JSON data')
    return value
