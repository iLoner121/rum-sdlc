"""Git workspace policies; no platform module depends on this adapter module."""
from pathlib import Path
from copy import deepcopy
import validation as v
import model as m
import git_validation as gv
import git_evidence as ge


def suggested_branch(topic):
    """A readable creation default, never a parser or evidence of association."""
    prefix = {'spike': 'chore'}.get(topic['type'], topic['type'])
    name = prefix + '/' + topic.get('short_id', topic['id']) + '-' + topic['slug']
    return gv.branch(name)


def target(state,params):
    topic=params.get('topic');line=params.get('line');name=params.get('branch')
    v.need(not(topic and line),'Choose Topic or Line, not both')
    if topic:
        t=m.topic(state,topic);topic=t['id'];v.need(t['status']!='closed','Unfinished Topic required')
        name = name if name is not None else suggested_branch(t)
        parent=state['lines'][m.relation(state,'line',topic)]
    elif line:
        parent=state['lines'].get(line);v.need(parent is not None and parent['status']!='closed','Open Line required')
        v.need(name is None or name==ge.line(state,parent['id'])['branch'],'Line branch differs from selected Line');name=ge.line(state,parent['id'])['branch']
    else:
        v.text(name,'Explicit branch for planning/integration workspace');parent=None
    gv.branch(name)
    return dict(branch=name,topic_id=topic,line_id=line,target_branch=ge.line(state,parent['id'])['branch'] if parent else None,default_base=ge.line(state,parent['id']).get('base_commit') if line else None)


def add_plan(state,params,facts):
    subject=target(state,params);path=Path(facts['path']);v.need(not facts['exists'] and facts['parent_exists'],'New directory must not exist and parent must exist')
    for item in facts['registered_paths']+[facts['metadata_path']]:
        other=Path(item);v.need(not(path.is_relative_to(other) or other.is_relative_to(path)),'Workspace overlaps existing tree or metadata')
    v.need(subject['branch'] not in facts['checked_out_branches'],'Branch already checked out; use/adopt existing workspace')
    if facts['branch_exists']:
        v.need(not params.get('base_ref'),'Existing branch keeps HEAD; do not supply base-ref');head=facts['branch_head']
    else:
        if subject['topic_id']:
            v.need(facts['first_code_allowed'],'Restore the recorded original Topic branch; first-code creation needs explicit inspection')
            v.need(facts['base_head'] in [facts['target_head'],*facts['dependency_heads']],'Topic baseline must be target or a declared dependency tip')
        else:v.need(bool(params.get('base_ref') or subject['default_base']),'New non-Topic branch requires an explicit baseline')
        head=facts['base_head']
    v.match(head,gv.SHA,'Actual workspace HEAD')
    return dict(subject,path=str(path),head=head,new_branch=not facts['branch_exists'],project_id=state['project']['id'],workflow_revision=state['revision'])


def same_store(binding, state, store_path):
    """One local binding identity rule for create, read and diagnosis."""
    v.need(store_path is not None, 'Store path required for binding identity checks')
    return (binding['project_id'] == state['project']['id'] and
            Path(binding['store']).expanduser().resolve() == Path(store_path).expanduser().resolve())


def health(observations,state=None,store_path=None):
    rows=deepcopy(observations);issues=[]
    for row in rows:
        errors=[];binding=row.get('binding')
        if not row.get('available'):errors.append('workspace unavailable')
        if row.get('binding_error'):errors.append(row['binding_error'])
        if binding:
            if binding['branch']!=row.get('branch_name'):errors.append('binding branch is stale')
            if state is not None:
                if not same_store(binding,state,store_path):errors.append('binding belongs to another Project or Store')
                elif binding['topic_id'] not in state['topics']:errors.append('bound Topic missing')
        if errors:issues.append(dict(path=row['path'],errors=errors))
        row['dirty']=None if row.get('binding_error') else bool(row.get('status'));row['issues']=errors
    return {'ok':not issues,'worktrees':rows,'issues':issues,'counts':{'total':len(rows),'dirty':sum(r['dirty'] is True for r in rows),'unavailable':sum(not r.get('available') for r in rows)},
            'limits':'Observed workspace/binding health; dirty files are preserved and not automatically repaired'}
