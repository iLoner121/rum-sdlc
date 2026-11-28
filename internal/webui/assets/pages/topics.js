import {el, icon, formatTime} from '../dom.js';
import {topicTable, documentView} from '../components.js';

export const outcomeNames = {'completed':'完成','cancelled':'取消','superseded':'被替代','carry-forward':'结转','backlog':'退回需求池'};
const stateNames = {planned:'待开始',active:'进行中',review:'待评审',ready:'待整合',paused:'已暂停',blocked:'受阻',closed:'已关闭'};
export function changeQuery(ctx, changes, page = ctx.route.page) {
  const query = new URLSearchParams(ctx.route.query);
  if(!Object.hasOwn(changes,'revision')) query.delete('revision');
  for (const [key,value] of Object.entries(changes)) value === '' || value == null ? query.delete(key) : query.set(key,String(value));
  ctx.navigate({page,projectID:ctx.route.projectID,query});
}
export function selectControl(label, values, value, onChange) {
  return el('label',{class:'select-control'},el('span',{class:'sr-only'},label),el('select',{'aria-label':label,onchange:e=>onChange(e.target.value)},values.map(([id,name])=>el('option',{value:id,selected:id===value},name))));
}
export function openTopic(ctx, row, view) {
  const query = {from:location.hash};
  if (view.basis === 'historical-results') {
    query.history_workset = ctx.route.query.get('workset_id');
    query.history_result = row.disposition || '';
  }
  ctx.navigate({page:'detail',projectID:ctx.route.projectID,entityID:row.id,query});
}
export function topicRows(ctx, rows, view, pool = false) {
  return topicTable(rows,{projectID:ctx.route.projectID,showPosition:pool,historical:view.basis==='historical-results',worksetID:ctx.route.query.get('workset_id'),onOpen:row=>openTopic(ctx,row,view)});
}
export function pagination(ctx, data, revision) {
  const offset=Number(data.offset)||0,limit=Number(data.limit)||30,total=Number(data.total)||0;
  return el('div',{class:'pagination'},el('span',{class:'muted'},total?`显示 ${offset+1}–${Math.min(offset+limit,total)}，共 ${total} 项`:'共 0 项'),el('div',{class:'pagination-controls'},selectControl('每页条数', [['30','30 / 页'],['100','100 / 页']],String(limit),v=>changeQuery(ctx,{limit:v,offset:0})),el('button',{class:'button',disabled:offset===0,onclick:()=>changeQuery(ctx,{offset:Math.max(0,offset-limit),revision})},'上一页'),el('span',{},`${Math.floor(offset/limit)+1} / ${Math.max(1,Math.ceil(total/limit))}`),el('button',{class:'button',disabled:offset+limit>=total,onclick:()=>changeQuery(ctx,{offset:offset+limit,revision})},'下一页')));
}
export async function renderList(ctx, pool = false) {
  const query=new URLSearchParams(ctx.route.query); if(pool)query.set('scope','pool');
  let view, failure, usingCache=false;
  try {
    if(query.get('scope')==='iteration'&&!query.get('workset_id')) {
      const choices=await ctx.api.project(ctx.route.projectID,'topics',{scope:'all',limit:1});
      const rounds=choices.data.filters?.iterations||[];
      const selected=rounds.find(r=>r.status==='active')||rounds[0];
      if(selected){query.set('workset_id',selected.id);view=await ctx.api.project(ctx.route.projectID,'topics',query);}
      else view={...choices,data:{...choices.data,rows:[],total:0,scope_total:0,offset:0,limit:30}};
    } else view=await ctx.api.project(ctx.route.projectID,'topics',query);
    ctx.state.lastView=view;
  } catch(error) {
    if(error.name==='AbortError')throw error;
    if(error.code==='stale'){query.delete('revision');query.set('offset','0');try{view=await ctx.api.project(ctx.route.projectID,'topics',query);ctx.state.lastView=view;failure='数据版本已变化，已回到第一页并保留筛选。';}catch(retryError){if(retryError.name==='AbortError'||!ctx.state.lastView)throw retryError;view=ctx.state.lastView;usingCache=true;failure=`重新读取失败，以下为上次成功数据：${retryError.message}`;}}
    else if(ctx.state.lastView){view=ctx.state.lastView;usingCache=true;failure=`读取失败，以下为上次成功数据：${error.message}`;} else throw error;
  }
  if(!usingCache&&query.toString()!==ctx.route.query.toString()){ctx.navigate({...ctx.route,query},{replace:true});return {element:el('p',{role:'status'},'正在更新浏览范围…')};}
  const data=view.data, rows=data.rows||[], historical=view.basis==='historical-results', scope=pool?'pool':query.get('scope')||'all';
  const root=el('section',{class:'page topics-page'});
  root.append(el('div',{class:'page-heading'},el('div',{},el('h1',{},pool?'需求池':'需求'),el('p',{class:'page-subtitle'},pool?'保留长期候选，按实际队列顺序浏览':historical?'该轮结束时的冻结结果，与当前需求信息分别呈现':'所有需求，保留完整生命周期'))));
  const refresh=()=>{const fresh=new URLSearchParams(ctx.route.query);fresh.delete('revision');ctx.navigate({...ctx.route,query:fresh},{replace:true});};
  root.querySelector('.page-heading').append(el('button',{class:'button',onclick:refresh},'刷新'));
  if(failure)root.append(el('div',{class:'notice warning',role:'status'},failure,el('button',{class:'button',onclick:refresh},'重新读取')));
  if(!pool){
    const tab=(title,value)=>el('button',{class:`tab ${scope===value?'selected':''}`,onclick:()=>changeQuery(ctx,{scope:value,workset_id:value==='iteration'?query.get('workset_id')||data.filters?.iterations?.[0]?.id:null,status:null,priority:null,line_id:null,milestone_id:null,offset:0,basis:null})},title);
    root.append(el('div',{class:'tabs'},tab('全部需求','all'),tab('迭代','iteration'),tab('需求池','pool')));
  }
  if(pool && data.pool) root.append(el('details',{class:'pool-description'},el('summary',{},'候选说明'),documentView(ctx,{table:'worksets',id:data.pool.id,field:'description',title:'候选说明'})));
  const filters=data.filters||{};
  if(scope==='iteration') root.append(el('div',{class:'scope-toolbar'},selectControl('指定迭代', (filters.iterations||[]).map(i=>[i.id,`${i.id} · ${i.name}`]),query.get('workset_id')||'',v=>changeQuery(ctx,{workset_id:v,status:null,priority:null,line_id:null,milestone_id:null,basis:null,offset:0})),historical?el('span',{class:'scope-label'},'冻结结果 · 不应用当前状态筛选'):selectControl('参与范围',[['scope','本轮范围'],['current','当前排队'],['history','全部参与（含移出）']],query.get('basis')||'scope',v=>changeQuery(ctx,{basis:v,offset:0}))));
  const search=el('input',{type:'search',class:'search-input',placeholder:'搜索编号或名称','aria-label':'搜索编号或名称',value:query.get('q')||''});
  const form=el('form',{class:'search-box',onsubmit:e=>{e.preventDefault();changeQuery(ctx,{q:search.value.trim(),offset:0});}},icon('search'),search,el('button',{class:'search-submit',type:'submit'},'搜索'));
  const toolbar=el('div',{class:'toolbar'},form);
  if(!historical){
    toolbar.append(selectControl('需求状态',[['','全部状态'],...Object.entries(stateNames)],query.get('status')||'',v=>changeQuery(ctx,{status:v,offset:0})));
    toolbar.append(selectControl('需求优先级',[['','全部优先级'],['P0','P0'],['P1','P1'],['P2','P2'],['P3','P3'],['unset','未设置']],query.get('priority')||'',v=>changeQuery(ctx,{priority:v,offset:0})));
    if(filters.lines?.length)toolbar.append(selectControl('开发线',[['','全部 Line'],...filters.lines.map(l=>[l.id,l.name||l.id])],query.get('line_id')||'',v=>changeQuery(ctx,{line_id:v,offset:0})));
    if(filters.milestones?.length&&!pool)toolbar.append(selectControl('里程碑',[['','全部里程碑'],...filters.milestones.map(m=>[m.id,`${m.id} · ${m.name}`])],query.get('milestone_id')||'',v=>changeQuery(ctx,{milestone_id:v,offset:0})));
  }
  toolbar.append(el('span',{class:'toolbar-count muted'},pool?`${data.total} 项匹配 / 池内 ${data.scope_total ?? data.total} 项`:`${data.total} 项`));root.append(toolbar);
  if(query.get('milestone_id'))root.append(el('div',{class:'scope-banner'},`里程碑范围：${query.get('milestone_id')}`,el('button',{class:'text-link',onclick:()=>changeQuery(ctx,{milestone_id:null,offset:0})},'清除范围')));
  if(rows.length)root.append(topicRows(ctx,rows,view,pool));
  else root.append(el('div',{class:'empty-state'},el('h2',{},query.get('q')||query.get('status')||query.get('priority')||query.get('line_id')?'没有匹配的需求':pool?'需求池暂无候选':scope==='iteration'&&!filters.iterations?.length?'尚无迭代':'此范围暂无需求'),el('p',{},'可切换范围或清除筛选；浏览不会改变需求归属。')));
  root.append(pagination(ctx,data,view.revision),el('footer',{class:'page-footer'},`本地数据 · ${formatTime(view.read_at)} · revision ${view.revision}`));
  return {element:root};
}
export async function render(ctx){return renderList(ctx,false);}
