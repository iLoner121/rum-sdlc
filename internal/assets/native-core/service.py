"""Construct a command envelope; only native Go executes transitions."""
def request(action, params, date, reason, evidence=None, decision=None):
    return dict(action=action, params=params, date=date, reason=reason,
                evidence=evidence, decision=decision)
