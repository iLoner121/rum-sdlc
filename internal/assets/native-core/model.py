"""Read-only adapter projections; authoritative validation lives in native Go."""
from native_bridge import call

def relation(state, kind, topic):
    row = state['relations'][kind].get(topic)
    return row['parent_id'] if row else None

def resolve(state, topic):
    return call('query', state=state, params={'view':'resolve', 'identifier':topic})

def validate(state):
    call('validate', state=state)
