"""Git extension contracts. Called only by Git/import adapters, never by core."""
import validation as v
import git_validation as gv
CODE={'baseline','previous_head','prepared_head','retained_ref'}

def line(state,identifier):
    value=state['lines'][identifier]['extensions'].get('git');v.obj(value,'Configure this Line with git_adapter line-configure first')
    v.need(set(value)=={'branch','base_commit'},'Invalid Git Line configuration');gv.branch(value['branch'])
    if value['base_commit'] is not None:v.match(value['base_commit'],gv.SHA,'Line base commit')
    return value

def context(value):
    data=value['extensions'].get('git');v.obj(data,'Git integration evidence');v.need(set(data)==CODE,'Incomplete Git integration evidence')
    v.match(data['baseline'],gv.SHA,'Selected baseline')
    v.need((data['previous_head'] is None)==(data['prepared_head'] is None),'Both source HEAD fields must be known or explicitly unstarted')
    for key in ('previous_head','prepared_head'):
        if data[key] is not None:v.match(data[key],gv.SHA,key)
    if data['retained_ref'] is not None:gv.durable(data['retained_ref'])
    return data

def retention_refs(state,topic):
    refs=[]
    def include(row):
        pending=row.get('integration_recheck') if isinstance(row,dict) else None
        ref=((pending.get('context') or {}).get('extensions',{}).get('git') or {}).get('retained_ref') if pending else None
        if ref is not None and ref not in refs:refs.append(ref)
    include(state['topics'].get(topic))
    for event in state['history']:
        for change in event['changes']:
            if change['path']==['topics',topic]:include(change['before']);include(change['after'])
    return refs

def closure(table,params):
    data=params.get('extensions',{}).get('git');v.obj(data,'Git closure evidence');outcome=params['outcome']
    if table=='topics':
        v.need(set(data) in ({'integration_commit','retained_ref'},{'integration_commit','retained_ref','no_code'}),'Invalid Git Topic closure evidence')
        if 'no_code' in data:
            v.need(outcome in {'delivered','research-complete'},'No-code completion applies only to delivery/research completion')
            v.text(data['no_code'],'Actual inspection confirming no Topic code exists')
            v.need(data['integration_commit'] is None and data['retained_ref'] is None,'No-code completion cannot claim a code commit or retained code')
        elif outcome in {'delivered','research-complete'}:
            v.match(data['integration_commit'],gv.SHA,'Integration commit');v.need(params['delivery_record']==data['integration_commit'],'Delivery record must match checked commit')
        else:v.need(data['integration_commit'] is None,'Non-delivery cannot claim integration');v.text(params.get('retention'),'Work disposition')
        if data['retained_ref'] is not None:gv.durable(data['retained_ref'])
    else:
        v.need(set(data)=={'integration_commit','expect_tip','retained_ref','last_release_ref','last_release_commit'},'Invalid Git Line closure evidence')
        if outcome=='integrated':
            for key in ('integration_commit','expect_tip'):v.match(data[key],gv.SHA,key)
            v.need(params['delivery_record']==data['integration_commit'],'Delivery record must match checked commit')
            v.need(all(data[k] is None for k in ('retained_ref','last_release_ref','last_release_commit')),'Incompatible Git integration fields')
        else:
            v.need(bool(params.get('retention'))==bool(data['retained_ref']),'Git retention evidence must match retention/discard choice')
            v.need(data['integration_commit'] is None and data['expect_tip'] is None,'Non-integrated closure cannot claim integration')
            if data['retained_ref'] is not None:gv.durable(data['retained_ref'])
            if outcome=='retired':gv.durable(data['retained_ref']);gv.durable(data['last_release_ref']);v.match(data['last_release_commit'],gv.SHA,'Last release commit')
            else:v.need(data['last_release_ref'] is None and data['last_release_commit'] is None,'Release facts only apply to retirement')
    return data
