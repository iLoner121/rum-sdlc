"""Git adapter syntax only; platform modules must not import this module."""
import re
import validation as v
SHA = r"(?:[0-9a-f]{40}|[0-9a-f]{64})"
text=v.text
need=v.need

def branch(value, label='branch'):
    text(value, label)
    need(value not in {'@', 'HEAD'} and not value.startswith(('-', '/')) and not value.endswith(('/', '.'))
         and not any(x in value for x in ('..', '@{', '//'))
         and not re.search(r'[\x00-\x20\x7f~^:?*\[\\]', value)
         and all(not part.startswith('.') and not part.endswith('.lock') for part in value.split('/')),
         'Invalid Git ' + label)
    return value



def durable(value,label='Retention ref'):
    v.text(value,label);v.need(value.startswith(('refs/heads/','refs/tags/')),'Durable branch/tag ref required');branch(value)
