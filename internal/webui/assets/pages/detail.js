import {el, clear, formatTime} from '../dom.js';
import {status, tabs, button, pageHeader, documentView, empty, errorNotice} from '../components.js';
const outcomes={completed:'完成',cancelled:'取消',superseded:'被替代','carry-forward':'结转',backlog:'退回需求池',delivered:'已交付','research-complete':'研究完成'};
const kinds={investigate:'调查',design:'设计',implement:'实现',verify:'验证',process:'流程处理'};
const fields={summary:'结论',verification:'验证证据',delivery_record:'交付记录',acceptance:'接受依据',retention:'保留记录',evidence:'证据',decision:'决定',reason:'原因',result:'结果',note:'说明',closed_on:'结束日期',date:'日期',outcome:'结束结果',successor:'后继',verified_on:'验证日期'};
function readable(value){
  if(value==null)return '—';
  if(typeof value!=='object')return String(value);
  if(Array.isArray(value))return value.map(readable).join('；');
  return Object.entries(value).map(([k,v])=>`${fields[k]||k}：${readable(v)}`).join('；');
}
function evidence(data){
  const list=el('dl',{class:'evidence-list'});
  for(const [key,value]of Object.entries(data||{})){
    if(value==null||key==='extensions')continue;
    list.append(el('dt',{},fields[key]||key),el('dd',{},key==='outcome'?(outcomes[value]||value):readable(value)));
  }
  return list;
}
function safeReturn(ctx){
  const from=ctx.route.query.get('from');
  const prefix=`#/projects/${encodeURIComponent(ctx.route.projectID)}/`;
  if(from?.startsWith(prefix)&&!from.includes('/topics/'+encodeURIComponent(ctx.route.entityID)))ctx.navigate(from);
  else ctx.navigate({page:'topics',projectID:ctx.route.projectID});
}
export async function render(ctx){
  let view,error;
  try{view=await ctx.api.project(ctx.route.projectID,`topics/${encodeURIComponent(ctx.route.entityID)}`);ctx.state.lastView=view;}
  catch(e){if(e.name==='AbortError')throw e;if(!ctx.state.lastView)throw e;view=ctx.state.lastView;error=e;}
  const data=view.data,topic=data.topic||{},rel=topic.status==='closed'?(topic.closure?.memberships||data.relations||{}):(data.relations||{}),root=el('section',{class:'page detail-page'});
  const copied=el('span',{class:'copy-feedback',role:'status'});
  root.append(pageHeader(topic.name||ctx.route.entityID,topic.short_id||view.short_id||ctx.route.entityID,el('div',{class:'heading-actions'},button('复制 ID',async()=>{try{await navigator.clipboard.writeText(topic.id||ctx.route.entityID);copied.textContent='已复制';}catch{copied.textContent=topic.id||ctx.route.entityID;}}),button('← 返回列表',()=>safeReturn(ctx)),copied)));
  if(error)root.append(errorNotice(error,()=>ctx.navigate({...ctx.route},{replace:true}),{staleAt:view.read_at}));
  root.append(el('div',{class:'detail-relations'},status(topic.status),el('span',{},`优先级 ${topic.priority || '未设置'}`),topic.status==='closed'?el('span',{class:'muted'},'关闭时归属'):null,el('span',{},`里程碑 ${rel.milestone_id||'未绑定'}`),el('span',{},`工作集合 ${rel.workset_id||'—'}`),el('span',{},`Line ${rel.line_id||'—'}`)));
  const historical=ctx.route.query.get('history_workset');
  if(historical){
    const banner=el('div',{class:'notice history-banner'},'正在核对历史来源…');root.append(banner);
    try{
      // Read the frozen record again. URL labels are navigation hints, never evidence.
      const past=await ctx.api.project(ctx.route.projectID,'topics',{scope:'iteration',workset_id:historical,q:topic.id||ctx.route.entityID,limit:100});
      const row=(past.data.rows||[]).find(r=>r.id===(topic.id||ctx.route.entityID));
      if(past.basis==='historical-results'&&row)clear(banner,el('strong',{},`${historical} 结束时：${outcomes[row.disposition]||row.disposition}`),el('p',{},row.note||''),el('p',{},'以下为该需求当前信息与当前正文，系统没有为本轮保存历史正文快照。'));
      else clear(banner,'未找到对应的冻结结果。以下仅展示当前需求信息。');
    }catch(e){if(e.name==='AbortError')throw e;clear(banner,`历史来源暂不可读：${e.message}。以下为当前需求信息。`);}
  }
  const tabItems=[{key:'context',label:'概述'},{key:'prd',label:'需求'},{key:'design',label:'设计'},{key:'acceptance',label:'验收'},{key:'tasks',label:'活动'},{key:'history',label:'历史与交付'}];
  const selected=tabItems.some(t=>t.key===ctx.route.query.get('tab'))?ctx.route.query.get('tab'):'context';
  root.append(tabs(tabItems,selected,key=>{const query=new URLSearchParams(ctx.route.query);query.set('tab',key);ctx.navigate({...ctx.route,query},{replace:true});}));
  if(['context','prd','design','acceptance'].includes(selected))root.append(documentView(ctx,{table:'topics',id:topic.id||ctx.route.entityID,field:selected,title:tabItems.find(t=>t.key===selected).label}));
  else if(selected==='tasks'){
    root.append(el('p',{class:'section-note'},'活动完成只表示该次工作结束，不等于需求验收或代码交付。'));
    if(!data.tasks?.length)root.append(empty('尚无活动','该需求尚未登记活动。'));
    for(const task of data.tasks||[]){
      const body=el('div',{class:'task-body'});let loaded=false;
      const entry=el('details',{class:'task-entry',ontoggle:e=>{if(e.currentTarget.open&&!loaded){loaded=true;body.append(documentView(ctx,{table:'tasks',id:task.id,field:'description',title:task.name}));}}},el('summary',{},el('span',{class:'task-number'},task.id?.split('/').pop()),el('span',{class:'task-title'},task.name),el('span',{class:'muted'},kinds[task.kind]||task.kind||'未分类'),el('span',{class:'task-state'},status(task.status)),el('span',{class:'task-chevron','aria-hidden':'true'},'›')),task.result?el('p',{class:'task-result'},readable(task.result)):null,body);
      root.append(entry);
    }
  }else{
    root.append(el('section',{class:'delivery-section'},el('h2',{},'交付与结束记录'),topic.closure?evidence(topic.closure):el('p',{class:'muted'},'此需求尚未结束；不据 Task 或 Git 状态推断已经交付。')));
    root.append(el('h2',{},'变更历史'));
    if(!data.history?.length)root.append(empty('暂无可用历史','未返回与该需求关联的历史记录。'));
    for(const item of data.history||[]){
      const req=item.request||item;
      root.append(el('details',{class:'history-entry'},el('summary',{},el('span',{},req.date||item.date||'日期未记录'),el('strong',{},req.action||item.action||'工作流记录'),el('span',{class:'muted'},item.revision?`revision ${item.revision}`:'')),evidence({reason:req.reason,evidence:req.evidence,decision:req.decision}),item.changes?evidence({result:item.changes}):null));
    }
  }
  root.append(el('footer',{class:'page-footer'},`本地数据 · ${formatTime(view.read_at)} · revision ${view.revision}`));
  return {element:root};
}
