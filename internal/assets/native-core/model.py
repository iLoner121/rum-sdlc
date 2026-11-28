"""Read-only adapter projections; authoritative validation lives in native Go."""
from native_bridge import call

def relation(state, kind, topic):
    row = state['relations'][kind].get(topic)
    return row['parent_id'] if row else None

def resolve(state, topic):
    return call('query', state=state, params={'view':'resolve', 'identifier':topic})

def validate(state):
    call('validate', state=state)


def topic(state, identifier):
    return call('query', state=state, params={'view':'show', 'table':'topics', 'identifier':identifier})['entity']

def topic_labels(state, identifiers):
    """Read presentation labels from the same snapshot used for binding checks."""
    labels = {}
    offset = 0
    while len(labels) < len(identifiers):
        page = call('query', state=state, params={
            'view': 'entities', 'table': 'topics', 'include_closed': True, 'offset': offset, 'limit': 200,
        })
        labels.update((row['id'], row['short_id']) for row in page['rows'] if row['id'] in identifiers)
        offset += len(page['rows'])
        if offset >= page['total']:
            break
    return labels
