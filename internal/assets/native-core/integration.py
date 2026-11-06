from native_bridge import call

def validate_context(value, topic, source, target):
    call('integration-context', value=value, topic=topic, source=source, target=target)
    return value

def validate_proof(value, topic, pending):
    call('integration-proof', value=value, topic=topic, pending=pending)
    return value
