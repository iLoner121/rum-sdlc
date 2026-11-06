#!/usr/bin/env python3
"""Outer Git integration. Platform records are read through Store; no workflow copies in code branches."""
import sys
sys.dont_write_bytecode = True
import agent_bootstrap
import agent_state
import repo_check
import os
import git_validation as gv
import argparse
import json
from pathlib import Path
import subprocess
import model as m
import git_workspaces as workspaces
import git_evidence as ge
from store import Store, atomic, safe, digest
import validation as v


class MutationUncertain(ValueError):
    """A write was attempted; an exception cannot prove no durable change occurred."""


def preview_context(root, store_path=None, target=None):
    def location(tree):
        return {'worktree':str(Path(tree).resolve()), 'git_dir':str(gitdir(tree)),
                'repository':str(gitdir(tree,True)), 'focus':str(focus_path(tree)),
                'index':str(index_path(tree))}
    return {'format':'rum-git-operation/1', 'caller':location(root),
            'target':location(target) if target is not None else None,
            'store':str(Store(store_path).root.resolve()) if store_path is not None else None}


def scoped_preview(root, store_path, result):
    """Outer target/fact token; the core token remains a portable state/request CAS."""
    snapshot = dict(result.get('code_snapshot') or {
        'format': 'rum-git-operation/2', 'request': result['request']})
    snapshot['context'] = preview_context(root, store_path)
    return dict(result, code_snapshot=snapshot, code_revision=digest(snapshot))


def git(root,*args):
    mutation = args and args[0] == 'worktree' and len(args) > 1 and args[1] in {'add','remove'}
    return repo_check.git(root,*args,readonly=not mutation,timeout=90 if mutation else 15).stdout.decode('utf-8').strip()


def repo(path):return Path(git(Path(path).expanduser(),'rev-parse','--show-toplevel')).resolve()


def gitdir(root,common=False):
    p=Path(git(root,'rev-parse','--git-common-dir' if common else '--absolute-git-dir'))
    return p.resolve() if p.is_absolute() else (root/p).resolve()


def focus_path(root):return agent_state.path(gitdir(root), 'focus.json', root)


def validate_focus(value):
    v.need(isinstance(value,dict) and set(value)=={'version','project_id','store','topic_id','branch'} and value['version'] in v.COMPATIBLE_VERSIONS,'Invalid local binding; preserve and reconcile explicitly')
    v.match(value['topic_id'],v.TOPIC,'Bound Topic');gv.branch(value['branch'])
    v.text(value['project_id'],'Project');v.text(value['store'],'Store')
    return value

def focus(root):
    p=focus_path(root)
    if not p.exists():
        v.need(not (gitdir(root)/'personal-workflow-4/focus.json').exists(),'Legacy adapter state detected; run migrate-state explicitly')
        return None
    value = v.loads(p.read_text())
    return None if value == {'unbound': True} else validate_focus(value)


def branch(root):return git(root,'symbolic-ref','--short','HEAD')

def commit(root,ref):return git(root,'rev-parse','--verify','--end-of-options',ref+'^{commit}')


def resolve(state,topic):
    result=m.resolve(state,topic);return dict(result,target_branch=ge.line(state,result['line_id'])['branch'])

def index_path(root):return agent_state.path(gitdir(root,True), 'topics.json', root)

def validate_index(data):
    v.need(isinstance(data,dict) and set(data)=={'version','topics'} and data['version'] in v.COMPATIBLE_VERSIONS,'Invalid Topic index; preserve it and reconcile explicitly')
    v.obj(data['topics'],'Repository Topic index')
    for key,rows in data['topics'].items():
        v.need(isinstance(key,str) and '/' in key,'Invalid index key')
        v.need(isinstance(rows,list) and bool(rows),'Invalid Topic index observations')
        for row in rows:
            v.need(isinstance(row,dict) and set(row)=={'ref','first_head'},'Invalid Topic ref observation');gv.durable(row['ref']);v.match(row['first_head'],gv.SHA,'First observed HEAD')
    return data

def topic_index(root):
    path=index_path(root)
    if not path.exists():
        v.need(not (gitdir(root,True)/'personal-workflow-4/topics.json').exists(),'Legacy adapter state detected; run migrate-state explicitly')
        return {'version':v.VERSION,'topics':{}}
    return validate_index(v.loads(path.read_text()))


def index_key(state,topic):return state['project']['id']+'/'+topic

def record_topic(root,store_path,topic):
    state=Store(store_path).read();data=topic_index(root);key=index_key(state,topic);ref='refs/heads/'+branch(root)
    rows=data['topics'].setdefault(key,[])
    if not any(row['ref']==ref for row in rows):rows.append({'ref':ref,'first_head':commit(root,ref)})
    path=index_path(root);path.parent.mkdir(parents=True,exist_ok=True);atomic(path,data)

def existing_ref(root,ref):
    return ref in git(root,'for-each-ref','--format=%(refname)',ref).splitlines()

def topic_ref(root,state,topic,explicit=None,required=True):
    """One actual local ref; never silently guess between recorded branches."""
    v.need(topic in state['topics'],'Missing Topic')
    if explicit is not None:
        ref=explicit if explicit.startswith('refs/') else 'refs/heads/'+explicit
        gv.durable(ref);v.need(existing_ref(root,ref),'Explicit Topic ref does not exist: '+ref)
        return {'ref':ref,'head':commit(root,ref)}
    recorded=[row['ref'] for row in topic_index(root)['topics'].get(index_key(state,topic),[])]
    candidates=list(recorded)
    for row in trees(root):
        if not Path(row['path']).is_dir() or 'prunable' in row:continue
        saved=focus(Path(row['path']))
        if saved and saved['project_id']==state['project']['id'] and saved['topic_id']==topic:
            v.need(saved['branch']==row.get('branch','').removeprefix('refs/heads/'),'Stale binding; inspect/rebind before resolving Topic code')
            candidates.append('refs/heads/'+saved['branch'])
    # Names never imply Topic ownership; creation suggestions are handled separately.
    live=sorted({ref for ref in candidates if existing_ref(root,ref)})
    v.need(len(live)<=1,'Ambiguous Topic code refs; explicitly select the actual ref: '+topic)
    if not live:
        v.need(not required,'Original Topic ref is missing; restore it or supply an explicit actual ref: '+topic);return None
    return {'ref':live[0],'head':commit(root,live[0])}

def line_configure(root,store_path,identifier,name,base_ref,date,reason,evidence,decision):
    from service import request
    state=Store(store_path).read();v.need(identifier in state['lines'],'Missing Line');gv.branch(name)
    for key,row in state['lines'].items():
        if key!=identifier:v.need(row['extensions'].get('git',{}).get('branch')!=name,'Git Line branches must be unique')
    line=state['lines'][identifier];old=line['extensions'].get('git')
    if old:v.need(old['branch']==name,'Configured Line branch is stable; preserve/reconcile existing code before changing its identity')
    base=commit(root,base_ref) if base_ref else None
    if line['type']=='mainline':v.need(existing_ref(root,'refs/heads/'+name),'Mainline branch must exist')
    else:v.need(base is not None,'Non-mainline Git configuration requires an actual base-ref')
    req=request('extension.set',dict(table='lines',id=identifier,namespace='git',value=dict(branch=name,base_commit=base)),date,reason,evidence,decision)
    return scoped_preview(root,store_path,dict(request=req,platform_preview=Store(store_path).execute(req,True)))

def current(root,store_path=None):
    saved=focus(root);store_path=store_path or (saved['store'] if saved else None)
    v.need(store_path is not None,'No project binding; supply --store or explicitly bind this worktree')
    agent_bootstrap.check_store(store_path)
    state=Store(store_path).read();actual=branch(root)
    if saved:
        v.need(workspaces.same_store(saved,state,store_path),'Binding belongs to another Project or Store; preserve work and explicitly unbind/rebind')
        v.need(saved['branch']==actual,'Stale local binding after branch switch; preserve work and explicitly unbind/rebind')
        identifier=saved['topic_id'];v.need(identifier in state['topics'],'Bound Topic missing from workflow')
        return dict(resolve(state,identifier),branch=actual,topic=state['topics'][identifier],bound=True)
    return {'project_id':state['project']['id'],'branch':actual,'mode':'planning-or-integration','topic_id':None,'binding_required':True}


def binding_plan(root,store_path,identifier):
    state=Store(store_path).read();v.need(identifier in state['topics'] and state['topics'][identifier]['status']!='closed','Bind an unfinished Topic')
    old=focus(root)
    v.need(old is None or (workspaces.same_store(old,state,store_path) and old['topic_id']==identifier),
           'Preserve previous code activity, then explicitly unbind before changing Project, Store or Topic')
    return {'version':v.VERSION,'project_id':state['project']['id'],'store':str(Store(store_path).root.resolve()),'topic_id':identifier,'branch':branch(root)}


def trees(root):
    rows=[];row=None
    for item in git(root,'worktree','list','--porcelain','-z').split('\0'):
        if not item:continue
        key,_,value=item.partition(' ')
        if key=='worktree':row={'path':value};rows.append(row)
        elif row is not None:row[key]=value or True
    return rows


def path_value(raw):
    p=Path(raw).expanduser().absolute()
    for x in [p,*p.parents]:v.need(not x.is_symlink(),'Worktree path cannot contain symlinks')
    return p.resolve()


def historical_code(state,topic):
    contexts=[]
    for event in state['history']:
        for change in event['changes']:
            if change['path']!=['topics',topic]:continue
            for row in (change['before'],change['after']):
                pending=row.get('integration_recheck') if isinstance(row,dict) else None
                if pending and 'git' in pending['context']['extensions']:contexts.append(ge.context(pending['context']))
                proof=(pending or {}).get('proof') or {}
                if proof.get('verification',{}).get('git',{}).get('verified_head') is not None:return True
                closure=row.get('closure') if isinstance(row,dict) else None
                if (closure or {}).get('extensions',{}).get('git',{}).get('integration_commit') is not None:return True
    return any(c['previous_head'] is not None for c in contexts)

def no_topic_code(root,state,topic,inspection,source_ref=None):
    """Local observations supplement, but cannot authenticate, cross-client inspection."""
    v.text(inspection,'Actual inspection confirming no Topic code exists')
    v.need(source_ref is None,'No-code completion cannot select a Topic source ref')
    v.need(not topic_index(root)['topics'].get(index_key(state,topic)) and not historical_code(state,topic),'Topic code was recorded; restore and verify it instead of claiming no code')
    v.need(topic_ref(root,state,topic,required=False) is None,'Topic code exists; use code verification/delivery')


def add_plan(root,args):
    state=Store(args.store).read();params=dict(vars(args))
    if args.topic and not args.branch:
        observed=topic_ref(root,state,args.topic,required=False)
        if observed:
            v.need(observed['ref'].startswith('refs/heads/'),'Topic workspace requires a branch');params['branch']=observed['ref'].removeprefix('refs/heads/')
    suggested = bool(args.topic and params.get('branch') is None)
    subject=workspaces.target(state,params);path=path_value(args.path);name=subject['branch']
    # A new tree must not contain or live within private adapter state either.
    agent_state.path(path, 'check', path)
    rows=trees(root);exists=existing_ref(root,'refs/heads/'+name)
    v.need(not (suggested and exists),'Suggested branch already exists; inspect it and explicitly select --branch, or choose another name. Names do not establish Topic ownership')
    target=commit(root,'refs/heads/'+subject['target_branch']) if subject['topic_id'] else None
    dependencies=[];selected=[];overrides={}
    for spec in getattr(args,'dependency_ref',[]) or []:
        key,sep,ref=spec.partition('=');v.need(sep and key not in overrides,'Use one --dependency-ref TOPIC=REF per dependency');overrides[key]=ref
    allowed=True;history=topic_index(root)
    if subject['topic_id']:
        tid=subject['topic_id'];topic=state['topics'][tid]
        v.need(set(overrides)<=set(topic['dependencies']),'Explicit ref is not a declared dependency')
        recorded=history['topics'].get(index_key(state,tid),[])
        known=bool(recorded) or historical_code(state,tid)
        # Existing code is always restored. Topic business status is not code history.
        allowed=not known and bool(getattr(args,'first_code',False) and getattr(args,'code_history_check',None))
        if not exists:
            v.need(allowed,'Original Topic code is recorded or first-code inspection is missing; restore the original branch, or use --first-code --code-history-check after confirming no prior code')
        if getattr(args,'code_history_check',None):v.need(getattr(args,'first_code',False),'code-history-check requires --first-code')
        if getattr(args,'first_code',False):v.need(not exists and not known,'First-code cannot recreate recorded code or restore an existing branch');v.text(getattr(args,'code_history_check',None),'Actual inspection confirming no prior Topic code')
    else:v.need(not overrides and not getattr(args,'first_code',False) and not getattr(args,'code_history_check',None),'Topic-only code options require --topic')
    base=None if exists else commit(root,args.base_ref or subject['default_base'] or ('refs/heads/'+subject['target_branch'] if subject['target_branch'] else 'HEAD'))
    if subject['topic_id'] and not exists and base!=target:
        for dep in state['topics'][subject['topic_id']]['dependencies']:
            observed=topic_ref(root,state,dep,overrides.get(dep));selected.append(dict(topic_id=dep,**observed));dependencies.append(observed['head'])
    else:
        for dep,ref in overrides.items():selected.append(dict(topic_id=dep,**topic_ref(root,state,dep,ref)))
    facts=dict(path=str(path),exists=path.exists(),parent_exists=path.parent.is_dir(),registered_paths=[str(Path(r['path']).resolve()) for r in rows],metadata_path=str(gitdir(root,True)),
               checked_out_branches=[r.get('branch','').removeprefix('refs/heads/') for r in rows],branch_exists=exists,branch_head=commit(root,'refs/heads/'+name) if exists else None,base_head=base,target_head=target,dependency_heads=dependencies,first_code_allowed=allowed)
    result=workspaces.add_plan(state,params,facts)
    return dict(result,dependency_refs=selected,code_index=history,first_code=bool(getattr(args,'first_code',False)),code_history_check=getattr(args,'code_history_check',None),target_head=target)

def inspect_workspaces(root,store_path=None):
    state=Store(store_path).read() if store_path else None;observed=[]
    for row in trees(root):
        item=dict(row,available=Path(row['path']).is_dir() and 'prunable' not in row,binding=None,binding_error=None,status=None,branch_name=row.get('branch','').removeprefix('refs/heads/'))
        if item['available']:
            try:
                check=repo_check.inspect(Path(row['path']));item['coverage']=check;item['status']='\n'.join(check['status_entries']);v.need(check['checks_complete'],'Incomplete repository checks: '+', '.join(check['unsupported_or_incomplete']));item['binding']=focus(Path(row['path']))
            except (ValueError,OSError) as e:item['binding_error']=str(e)
        observed.append(item)
    result=workspaces.health(observed,state,store_path)
    if state is not None:result['platform']={'ok':True,'project_id':state['project']['id'],'revision':state['revision']}
    return result


def remove_plan(root,args):
    path=path_value(args.path);rows=[r for r in trees(root) if Path(r['path']).resolve()==path]
    v.need(len(rows)==1,'Not one registered worktree');row=rows[0]
    v.need(path!=root and gitdir(path)!=gitdir(path,True),'Cannot remove calling or primary worktree')
    v.need(not ('locked' in row or 'prunable' in row),'Locked/unavailable worktree')
    check=repo_check.inspect(path);v.need(check['checks_complete'],'Incomplete checks; follow git-fallback.md before cleanup');v.need(not check['dirty'],'Preserve tracked, untracked and ignored files first')
    head=commit(path,'HEAD');v.need(head==args.expect_head,'Stale expected HEAD')
    retained=args.retain_ref or row.get('branch');v.need(isinstance(retained,str) and retained.startswith(('refs/heads/','refs/tags/')),'Need a durable retention ref')
    tip=commit(root,retained);git(root,'merge-base','--is-ancestor',head,tip);v.text(args.resources_check,'Actual process/port/data cleanup check')
    return {'path':str(path),'head':head,'retain_ref':retained,'retained_tip':tip,'resources_check':args.resources_check}


def delivery(root,store_path,topic,ref,source_ref=None,squash_evidence=None):
    state=Store(store_path).read();v.need(topic in state['topics'],'Missing Topic');resolved=resolve(state,topic)
    integrated=commit(root,ref);target=commit(root,'refs/heads/'+resolved['target_branch'])
    git(root,'merge-base','--is-ancestor',integrated,target)
    selected=topic_ref(root,state,topic,source_ref);source=selected['head']
    try:git(root,'merge-base','--is-ancestor',source,integrated);preserved=True
    except ValueError:
        v.text(squash_evidence,'Approved squash criteria and final-diff evidence; see closure.md');preserved=False
    return dict(resolved,integration_commit=integrated,target_ref='refs/heads/'+resolved['target_branch'],target_tip=target,source_ref=selected['ref'],source_tip=source,source_history_preserved=preserved,squash_evidence=squash_evidence,
                limits='Read-only code ancestry evidence; no acceptance or content correctness proof')


def source_code(root,state,topic,source_ref=None):
    found=topic_ref(root,state,topic,source_ref,required=False)
    if found is None:
        v.need(not topic_index(root)['topics'].get(index_key(state,topic)) and not historical_code(state,topic),'Restore original recorded Topic branch before retargeting');return None
    v.need(found['ref']=='refs/heads/'+branch(root),'Inspect the actual Topic code workspace for retargeting')
    v.need(not git(root,'status','--porcelain','--untracked-files=all'),'Preserve/commit dirty product files before code retarget verification')
    return found['head']

def preserved(root,head,current,retained_refs):
    try:git(root,'merge-base','--is-ancestor',head,current);return
    except ValueError:pass
    refs=[retained_refs] if isinstance(retained_refs,str) else (retained_refs or [])
    for ref in refs:
        if not isinstance(ref,str) or not ref.startswith(('refs/heads/','refs/tags/')):continue
        try:
            tip=commit(root,ref);git(root,'merge-base','--is-ancestor',head,tip);return
        except ValueError:continue
    raise ValueError('Original code needs a real durable retention ref containing '+head)


def retarget_plan(root,store_path,topic,to_line,date,reason,evidence,decision,dependencies,contracts,previous_head=None,retained_ref=None,source_ref=None):
    from service import request
    from integration import validate_context
    from git_evidence import retention_refs
    state=Store(store_path).read();v.need(topic in state['topics'] and to_line in state['lines'],'Missing Topic or target Line')
    current=m.relation(state,'line',topic);refresh=current==to_line;old=None
    if refresh:
        pending=state['topics'][topic]['integration_recheck'];v.need(pending is not None,'Target is unchanged and there is no integration context to refresh')
        old=ge.context(pending['context'])
        if old['previous_head'] is not None:
            v.need(previous_head is None or previous_head==old['previous_head'],'Refresh must preserve the original source HEAD anchor');previous_head=old['previous_head']
        if retained_ref is None:retained_ref=old['retained_ref']
    refs=retention_refs(state,topic)
    if retained_ref is not None:
        from git_validation import durable
        durable(retained_ref,'Retained source ref');commit(root,retained_ref);refs=[retained_ref,*refs]
    baseline=commit(root,'refs/heads/'+ge.line(state,to_line)['branch']);prepared=source_code(root,state,topic,source_ref)
    if prepared is not None:
        v.match(previous_head,gv.SHA,'Explicit original Topic HEAD');previous_head=commit(root,previous_head)
        git(root,'merge-base','--is-ancestor',baseline,prepared);preserved(root,previous_head,prepared,refs)
        if old and old['prepared_head'] is not None:preserved(root,old['prepared_head'],prepared,refs)
    else:v.need(previous_head is None and retained_ref is None,'Unstarted Topic has no source history to retain')
    context=dict(topic_id=topic,from_line_id=pending['context']['from_line_id'] if old else current,line_id=to_line,dependencies=dependencies,contracts=contracts,extensions={'git':dict(baseline=baseline,previous_head=previous_head,prepared_head=prepared,retained_ref=retained_ref)})
    validate_context(context,topic,context['from_line_id'],to_line);ge.context(context)
    params=dict(id=topic,context=context) if refresh else dict(kind='line',topic_id=topic,to=to_line,integration_context=context)
    req=request('topic.integration.prepare' if refresh else 'relation.move',params,date,reason,evidence,decision)
    preview=Store(store_path).execute(req,True)
    return scoped_preview(root,store_path,{'request':req,'platform_preview':preview,'limits':'Read-only preparation; inspect code/design impact, then apply the exact request with both platform and code preview tokens'})


def retarget_verify(root,store_path,topic,date,report,reason,evidence,decision,source_ref=None,no_code=None):
    from service import request
    from git_evidence import retention_refs
    state=Store(store_path).read();v.need(topic in state['topics'],'Missing Topic');pending=state['topics'][topic]['integration_recheck']
    v.need(pending is not None,'No selected target revalidation context');context=pending['context'];code=ge.context(context)
    target=commit(root,'refs/heads/'+ge.line(state,context['line_id'])['branch'])
    v.need(target==code['baseline'],'Target Line tip changed; pause and refresh the integration context before verification')
    if no_code is not None:
        v.need(code['previous_head'] is None and code['prepared_head'] is None,'Recorded source code requires code verification')
        no_topic_code(root,state,topic,no_code,source_ref);verification={'no_code':no_code}
    else:
        head=source_code(root,state,topic,source_ref)
        v.need(head is not None,'No Topic code: use --no-code with actual inspection, or create the intended code branch before verifying');git(root,'merge-base','--is-ancestor',code['baseline'],head)
        for key in ('previous_head','prepared_head'):
            if code[key] is not None:preserved(root,code[key],head,retention_refs(state,topic))
        verification={'verified_head':head}
    proof=dict(context,verification={'git':verification},verified_on=date,report=report)
    req=request('topic.state',dict(id=topic,status='review',verification=report,integration_proof=proof),date,reason,evidence,decision)
    preview=Store(store_path).execute(req,True)
    return scoped_preview(root,store_path,{'request':req,'platform_preview':preview,'limits':'Actual baseline/HEAD facts only; report must cite completed affected tests, contracts and acceptance checks; apply with both preview tokens'})


def apply_integration(root,store_path,req,expected,source_ref=None,expected_code=None):
    # Re-observe external facts immediately before the normal platform CAS write.
    if req.get('action')=='relation.move' and req['params'].get('kind')=='line':
        p=req['params'];c=p['integration_context'];g=ge.context(c)
        fresh=retarget_plan(root,store_path,p['topic_id'],p['to'],req['date'],req['reason'],req['evidence'],req['decision'],c['dependencies'],c['contracts'],g['previous_head'],g['retained_ref'],source_ref)
    elif req.get('action')=='topic.integration.prepare':
        p=req['params'];c=p['context'];g=ge.context(c)
        fresh=retarget_plan(root,store_path,p['id'],c['line_id'],req['date'],req['reason'],req['evidence'],req['decision'],c['dependencies'],c['contracts'],g['previous_head'],g['retained_ref'],source_ref)
    elif req.get('action')=='topic.state' and req['params'].get('status')=='review' and req['params'].get('integration_proof'):
        p=req['params'];inspection=p['integration_proof'].get('verification',{}).get('git',{}).get('no_code')
        fresh=retarget_verify(root,store_path,p['id'],req['date'],p['verification'],req['reason'],req['evidence'],req['decision'],source_ref,inspection)
    elif req.get('action')=='extension.set' and req['params'].get('table')=='lines' and req['params'].get('namespace')=='git':
        p=req['params'];cfg=p['value'];fresh=line_configure(root,store_path,p['id'],cfg['branch'],cfg['base_commit'],req['date'],req['reason'],req['evidence'],req['decision'])
    elif req.get('action')=='line.state':
        p=req['params'];observations=p.get('observations',{}).get('git');v.obj(observations,'Git Line state observations');v.need(set(observations)=={'observed_tip'},'Invalid Git Line state observations')
        fresh=line_state_plan(root,store_path,p['id'],p['status'],p.get('resume_when'),observations['observed_tip'],req['date'],req['reason'],req['evidence'],req['decision'])
    elif req.get('action') in {'topic.close','line.close','closure.correct'}:
        fresh=closure_plan(root,store_path,req,source_ref)
    else:raise ValueError('Unsupported Git evidence request')
    v.need(expected_code == fresh['code_revision'],
           'Stale/missing code preview; Store, repository/worktree, adapter state paths, request or relevant code facts changed. Revalidate and generate a fresh preview for this operation')
    v.need(fresh['request']==req,'Code facts changed since inspection; generate and inspect fresh evidence')
    try:
        return Store(store_path).execute(req,expect=expected)
    except (ValueError,OSError) as error:
        raise MutationUncertain(str(error)) from error


def line_state_plan(root,store_path,identifier,status,resume_when,expected_tip,date,reason,evidence,decision):
    from service import request
    state=Store(store_path).read();v.need(identifier in state['lines'],'Missing Line')
    tip=commit(root,'refs/heads/'+ge.line(state,identifier)['branch'])
    v.match(expected_tip,gv.SHA,'Explicit expected Line tip');v.need(tip==expected_tip,'Line tip changed')
    params=dict(id=identifier,status=status,observations={'git':{'observed_tip':tip}})
    if resume_when is not None:params['resume_when']=resume_when
    req=request('line.state',params,date,reason,evidence,decision)
    return scoped_preview(root,store_path,dict(request=req,platform_preview=Store(store_path).execute(req,True)))

def line_retention(root,store_path,identifier,retained_ref,last_release_ref=None):
    state=Store(store_path).read();v.need(identifier in state['lines'],'Missing Line');line=state['lines'][identifier]
    from git_validation import durable
    durable(retained_ref,'Retained code ref');source=commit(root,'refs/heads/'+ge.line(state,line['id'])['branch']);retained=commit(root,retained_ref);git(root,'merge-base','--is-ancestor',source,retained)
    result=dict(line_id=identifier,source_tip=source,retained_ref=retained_ref,retained_tip=retained)
    if line['type']=='maintenance':
        durable(last_release_ref,'Last release ref');release=commit(root,last_release_ref);git(root,'merge-base','--is-ancestor',release,source);result.update(last_release_ref=last_release_ref,last_release_commit=release)
    else:v.need(last_release_ref is None,'Last release only applies to maintenance retirement')
    return result


def closure_plan(root,store_path,req,source_ref=None):
    state=Store(store_path).read();p=req['params']
    v.need(req['action'] in {'topic.close','line.close','closure.correct'},'Closure/correction request required')
    if req['action']=='closure.correct':
        table=p['table'];v.need(table in {'topics','lines'},'Git closure corrections apply to Topics/Lines')
        row=state[table][p['id']];v.need(row['closure'] is not None,'Closed record required')
        preview=Store(store_path).execute(req,True)
        old=row['closure'];p=dict(old,**p['changes'],id=p['id'])
        # General business facts remain editable after authorized source cleanup.
        # Changing any code-related fact still runs the full external checks.
        code_keys={'delivery_record','retention','discard_decision','last_release'}
        changed=any(old.get(key)!=p.get(key) for key in code_keys)
        for namespace in ('git','git-review'):
            changed=changed or old.get('extensions',{}).get(namespace)!=p.get('extensions',{}).get(namespace)
        if not changed:
            return scoped_preview(root,store_path,dict(request=req,platform_preview=preview,git_observations={},limits='General fact correction; target identity checked, no source/target code evidence required'))
    else:table='topics' if req['action']=='topic.close' else 'lines'
    data=ge.closure(table,p);outcome=p['outcome'];observed={} 
    if table=='topics':
        if 'no_code' in data:
            no_topic_code(root,state,p['id'],data['no_code'],source_ref);observed={'no_code':data['no_code']}
        elif outcome in {'delivered','research-complete'}:
            observed=delivery(root,store_path,p['id'],data['integration_commit'],source_ref,p.get('extensions',{}).get('git-review',{}).get('squash_evidence'))
            if data['retained_ref'] is not None:
                tip=commit(root,data['retained_ref']);git(root,'merge-base','--is-ancestor',observed['source_tip'],tip)
                observed.update(retained_ref=data['retained_ref'],retained_tip=tip)
        elif data['retained_ref'] is not None:
            selected=topic_ref(root,state,p['id'],source_ref);source=selected['head'];tip=commit(root,data['retained_ref']);git(root,'merge-base','--is-ancestor',source,tip);observed=dict(source_ref=selected['ref'],source_tip=source,retained_ref=data['retained_ref'],retained_tip=tip)
        else:
            v.text(req['decision'],'Explicit code discard/no-code decision')
            selected=topic_ref(root,state,p['id'],source_ref,required=False)
            if selected is None:
                # Missing recorded code is not an unstarted Topic. Cross-client
                # no-code facts still require the agent's actual inspection.
                no_topic_code(root,state,p['id'],req['decision'],source_ref)
            observed={'source_ref':selected['ref'] if selected else None,
                      'source_tip':selected['head'] if selected else None}
    elif outcome=='integrated':
        line=state['lines'][p['id']];v.need(line['type']=='development','Only development Lines integrate wholesale')
        source=commit(root,'refs/heads/'+ge.line(state,p['id'])['branch']);v.need(source==data['expect_tip'],'Line tip changed')
        integrated=commit(root,data['integration_commit']);target=commit(root,'refs/heads/'+ge.line(state,state['project']['default_line_id'])['branch'])
        git(root,'merge-base','--is-ancestor',source,integrated);git(root,'merge-base','--is-ancestor',integrated,target);observed=dict(source_ref='refs/heads/'+ge.line(state,p['id'])['branch'],source_tip=source,target_ref='refs/heads/'+ge.line(state,state['project']['default_line_id'])['branch'],target_tip=target,integration_commit=integrated)
    elif data['retained_ref'] is not None:
        observed=line_retention(root,store_path,p['id'],data['retained_ref'],data['last_release_ref'])
        if outcome=='retired':v.need(observed['last_release_commit']==data['last_release_commit'],'Last release facts changed')
    else:
        v.text(req['decision'],'Explicit code discard decision')
        ref='refs/heads/'+ge.line(state,p['id'])['branch']
        v.need(existing_ref(root,ref),'Original Line ref is missing; restore it or follow git-fallback.md with actual evidence')
        observed={'source_ref':ref,'source_tip':commit(root,ref)}
    # Keep Git freshness in the adapter: the pinned core token protects only workflow data.
    preview=Store(store_path).execute(req,True)
    snapshot={'format':'rum-git-closure/1', 'repository':str(gitdir(root,True)),
              'store':str(Store(store_path).root.resolve()), 'project_id':state['project']['id'],
              'request':req, 'observations':observed}
    return scoped_preview(root,store_path,dict(request=req,platform_preview=preview,git_observations=observed,
                code_snapshot=snapshot,
                limits='Code snapshot freshness only, not tests or approval. Revalidate changed targets; keep one Git writer through apply.'))

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--version',action='version',version=v.VERSION)
    subs=p.add_subparsers(dest='command',required=True)
    for name in ('current','bind','unbind','list','add','adopt','remove','delivery','line-delivery','retain-check','check','retarget-plan','retarget-verify','integration-apply','line-retention','line-configure','closure-plan','line-state-plan','migrate-state'):
        c=subs.add_parser(name);c.add_argument('--repo',required=True);c.add_argument('--state-home')
        if name in {'current','bind','add','adopt','delivery','line-delivery','list','check','retarget-plan','retarget-verify','integration-apply','line-retention','line-configure','closure-plan','line-state-plan'}:c.add_argument('--store',required=name not in {'current','list','check'})
        if name in {'bind','adopt','delivery','retarget-plan','retarget-verify'}:c.add_argument('--topic',required=True)
        if name in {'add','adopt','remove'}:c.add_argument('--path',required=True)
        if name in {'bind','unbind','add','adopt','remove','migrate-state'}:c.add_argument('--dry-run',action='store_true');c.add_argument('--expect-revision')
        if name=='add':
            c.add_argument('--topic');c.add_argument('--line');c.add_argument('--base-ref');c.add_argument('--branch');c.add_argument('--dependency-ref',action='append',default=[]);c.add_argument('--first-code',action='store_true');c.add_argument('--code-history-check')
        if name=='remove':c.add_argument('--expect-head',required=True);c.add_argument('--retain-ref');c.add_argument('--resources-check',required=True)
        if name in {'delivery','line-delivery'}:c.add_argument('--commit',required=True)
        if name=='delivery':c.add_argument('--source-ref');c.add_argument('--squash-evidence')
        if name=='line-delivery':c.add_argument('--line',required=True);c.add_argument('--expect-tip',required=True)
        if name in {'retarget-plan','retarget-verify'}:
            for field in ('date','reason','evidence','decision'):c.add_argument('--'+field,required=True)
            c.add_argument('--source-ref')
        if name=='retarget-plan':
            for field in ('to-line','dependencies','contracts'):c.add_argument('--'+field,required=True)
            c.add_argument('--previous-head');c.add_argument('--retained-ref')
        if name=='line-retention':
            c.add_argument('--line',required=True);c.add_argument('--retained-ref',required=True);c.add_argument('--last-release-ref')
        if name=='line-state-plan':
            for field in ('line','status','expect-tip','date','reason'):c.add_argument('--'+field,required=True)
            c.add_argument('--resume-when');c.add_argument('--evidence');c.add_argument('--decision')
        if name=='line-configure':
            for field in ('line','branch','date','reason','evidence','decision'):c.add_argument('--'+field,required=True)
            c.add_argument('--base-ref')
        if name=='closure-plan':c.add_argument('--request',required=True);c.add_argument('--source-ref')
        if name=='integration-apply':
            c.add_argument('--request',required=True);c.add_argument('--expect-revision',required=True);c.add_argument('--source-ref');c.add_argument('--expect-code-revision',help='Required code_revision from this operation preview; pins Store, Git target and relevant facts for every integration-apply')
        if name=='retarget-verify':c.add_argument('--report',required=True);c.add_argument('--no-code',help='Actual inspection confirming this Topic has no code')
        if name=='retain-check':c.add_argument('--ref',required=True);c.add_argument('--contains',required=True)
    a=p.parse_args()
    agent_bootstrap.check_store(getattr(a, 'store', None))
    root=repo(a.repo)
    agent_state.configure(a.state_home)
    # Validate every registered tree before creating even an external lock file.
    for row in trees(root): agent_state.path(Path(row['path']), 'check', root)
    agent_state.path(gitdir(root,True),'check',root)
    with agent_state.locked():
        return execute(a,root)


def execute(a,root):
    if a.command == 'migrate-state':
        old_index=gitdir(root,True)/'personal-workflow-4/topics.json';old_focus=gitdir(root)/'personal-workflow-4/focus.json'
        plan={}
        for old,validator in [(old_index,validate_index),(old_focus,validate_focus)]:
            if old.exists():
                v.need(not any(p.is_symlink() for p in [old,*old.parents]),'Legacy state symlink')
                plan[str(old)]=validator(v.loads(old.read_text()))
        context=preview_context(root)
        token=digest({'command':a.command,'context':context,'plan':plan})
        if a.dry_run: result={'changed':False,'revision':token,'context':context,'legacy_files':list(plan),'legacy_retained':True}
        else:
            v.need(a.expect_revision==token,'Stale/missing migration preview')
            try:
                result=agent_state.migrate(old_index,old_focus,index_path(root),focus_path(root),validate_index,validate_focus)
            except (ValueError,OSError) as error:
                raise MutationUncertain(str(error)) from error
        print(json.dumps(result,indent=2));return 0
    if a.command in {'add','remove','delivery','line-delivery','retain-check','retarget-plan','retarget-verify','integration-apply','line-retention','line-configure','closure-plan','line-state-plan'}:
        check=repo_check.inspect(root)
        v.need(check['checks_complete'],'Incomplete repository coverage; follow git-fallback.md: '+', '.join(check['unsupported_or_incomplete']))
    if a.command=='line-configure':result=line_configure(root,a.store,a.line,a.branch,a.base_ref,a.date,a.reason,a.evidence,a.decision)
    elif a.command=='line-state-plan':result=line_state_plan(root,a.store,a.line,a.status,a.resume_when,a.expect_tip,a.date,a.reason,a.evidence,a.decision)
    elif a.command=='closure-plan':result=closure_plan(root,a.store,v.loads(Path(a.request).read_text()),a.source_ref)
    elif a.command=='current':result=current(root,a.store)
    elif a.command in {'list','check'}:result=inspect_workspaces(root,a.store)
    elif a.command=='line-retention':result=line_retention(root,a.store,a.line,a.retained_ref,a.last_release_ref)
    elif a.command=='integration-apply':result=apply_integration(root,a.store,v.loads(Path(a.request).read_text()),a.expect_revision,a.source_ref,a.expect_code_revision)
    elif a.command=='retarget-plan':result=retarget_plan(root,a.store,a.topic,a.to_line,a.date,a.reason,a.evidence,a.decision,a.dependencies,a.contracts,a.previous_head,a.retained_ref,a.source_ref)
    elif a.command=='retarget-verify':result=retarget_verify(root,a.store,a.topic,a.date,a.report,a.reason,a.evidence,a.decision,a.source_ref,a.no_code)
    elif a.command=='delivery':result=delivery(root,a.store,a.topic,a.commit,a.source_ref,a.squash_evidence)
    elif a.command=='line-delivery':
        state=Store(a.store).read();line=state['lines'][a.line];v.need(line['type']=='development','Only development Lines integrate wholesale')
        tip=commit(root,'refs/heads/'+ge.line(state,line['id'])['branch']);v.need(tip==a.expect_tip,'Line tip changed')
        sha=commit(root,a.commit);target=commit(root,'refs/heads/'+ge.line(state,state['project']['default_line_id'])['branch'])
        git(root,'merge-base','--is-ancestor',tip,sha);git(root,'merge-base','--is-ancestor',sha,target)
        result={'line_id':line['id'],'source_tip':tip,'integration_commit':sha,'target_tip':target}
    elif a.command=='retain-check':
        v.need(a.ref.startswith(('refs/heads/','refs/tags/')),'Durable branch/tag ref required');head=commit(root,a.contains);tip=commit(root,a.ref)
        git(root,'merge-base','--is-ancestor',head,tip);result={'retained_ref':a.ref,'contains':head,'tip':tip}
    else:
        if a.command in {'bind','adopt'}:
            target=root if a.command=='bind' else path_value(a.path)
            v.need(gitdir(target,True)==gitdir(root,True),'Cannot adopt another repository')
            if a.command=='adopt':
                matches=[row for row in trees(root) if Path(row['path']).resolve()==target]
                v.need(len(matches)==1,'Adopt requires an exact registered workspace root')
                v.need(not ('locked' in matches[0] or 'prunable' in matches[0]),'Locked/unavailable workspace cannot be adopted')
            plan=binding_plan(target,a.store,a.topic)
        elif a.command=='unbind':plan={'before':focus(root),'path':str(focus_path(root))}
        elif a.command=='add':plan=add_plan(root,a)
        else:plan=remove_plan(root,a)
        facts = {'head': commit(target,'HEAD'), 'index': topic_index(target), 'before_binding':focus(target)} if a.command in {'bind','adopt'} else None
        selected=target if a.command in {'bind','adopt'} else Path(plan['path']) if a.command=='remove' else None
        context=preview_context(root,getattr(a,'store',None),selected)
        token=digest({'command':a.command,'context':context,'plan':plan,'facts':facts})
        if a.dry_run:result={'changed':False,'revision':token,'context':context,'plan':plan}
        else:
            v.need(a.expect_revision==token,'Stale/missing preview token; inspect with dry-run first')
            try:
                if a.command in {'bind','adopt'}:
                    file=focus_path(target);file.parent.mkdir(parents=True,exist_ok=True);atomic(file,plan);record_topic(target,a.store,a.topic)
                elif a.command=='unbind':
                    file=focus_path(root)
                    file.parent.mkdir(parents=True,exist_ok=True)
                    atomic(file, {'unbound': True})
                elif a.command=='add':
                    arguments=['worktree','add']+(['-b',plan['branch']] if plan['new_branch'] else [])+['--',plan['path'],plan['head'] if plan['new_branch'] else plan['branch']]
                    git(root,*arguments);target=Path(plan['path']);v.need(branch(target)==plan['branch'] and commit(target,'HEAD')==plan['head'],'Checkout changed during creation')
                    if a.topic:
                        file=focus_path(target);file.parent.mkdir(parents=True,exist_ok=True);atomic(file,binding_plan(target,a.store,a.topic));record_topic(target,a.store,a.topic)
                else:
                    old_focus=focus_path(Path(plan['path']))
                    git(root,'worktree','remove','--',plan['path'])
                    if old_focus.exists():old_focus.unlink()
            except (ValueError,OSError) as error:
                print(json.dumps({'changed':None,'error':str(error),'recovery':'Inspect directory, Git registration and local binding; preserve partial results, repair then adopt. No reset/delete retry.'}),file=sys.stderr);return 1
            result={'changed':True,'plan':plan,'committed':False,'workflow_modified':False}
    print(json.dumps(result,ensure_ascii=False,indent=2));return 1 if a.command=='check' and not result['ok'] else 0


if __name__=='__main__':
    try:sys.exit(main())
    except MutationUncertain as error:
        print(json.dumps({'ok':False,'error':str(error),'changed':None,
              'recovery':'A write was attempted. Inspect current Store and adapter files; preserve partial results before a fresh preview or idempotent migration retry.'},ensure_ascii=False),file=sys.stderr);sys.exit(1)
    except (ValueError,OSError,KeyError,TypeError,UnicodeError) as error:
        print(json.dumps({'ok':False,'error':str(error),'changed':False},ensure_ascii=False),file=sys.stderr);sys.exit(1)
