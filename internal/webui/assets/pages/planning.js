import {el,formatTime} from '../dom.js';
import {status,button,pageHeader,documentView,empty,errorNotice} from '../components.js';
const resultNames={completed:'完成',replanned:'重新规划',cancelled:'取消',superseded:'被替代','carry-forward':'结转',backlog:'退回需求池'};
async function loadAll(ctx,endpoint){
  let first=await ctx.api.project(ctx.route.projectID,endpoint,{limit:200}),rows=[...(first.data.rows||[])];
  while(rows.length<first.data.total){
    const next=await ctx.api.project(ctx.route.projectID,endpoint,{limit:200,offset:rows.length,revision:first.revision});
    if(!next.data.rows?.length)break;rows.push(...next.data.rows);
  }
  return {...first,data:{...first.data,rows}};
}
export async function renderPlanning(ctx,iteration=false){
  const endpoint=iteration?'iterations':'milestones',table=iteration?'worksets':'milestones';
  let view,readError;
  try{try{view=await loadAll(ctx,endpoint);}catch(e){if(e.code!=='stale')throw e;view=await loadAll(ctx,endpoint);}ctx.state.lastView=view;}
  catch(e){if(e.name==='AbortError')throw e;if(!ctx.state.lastView)throw e;view=ctx.state.lastView;readError=e;}
  const rows=view.data.rows||[],selectedID=ctx.route.query.get('selected')||ctx.state.selected;
  const selected=rows.find(r=>r.id===selectedID)||rows.find(r=>r.status==='active')||rows[0];
  const root=el('section',{class:'page planning-page'},pageHeader(iteration?'迭代':'里程碑',iteration?'逐轮回顾范围、目标与交付结果':'按阶段顺序查看目标与需求范围'));
  if(readError)root.append(errorNotice(readError,()=>ctx.navigate({...ctx.route},{replace:true}),{staleAt:view.read_at}));
  if(!rows.length){root.append(empty(iteration?'尚无迭代':'尚无里程碑','此项目尚未建立对应规划。浏览不会创建或启动规划。'));return{element:root};}
  const list=el('nav',{class:'planning-list','aria-label':iteration?'迭代列表':'里程碑列表'});
  for(const row of rows){
    list.append(el('button',{class:`planning-item ${selected.id===row.id?'selected':''}`,'aria-current':selected.id===row.id?'true':null,onclick:()=>{ctx.state.selected=row.id;const query=new URLSearchParams(ctx.route.query);query.set('selected',row.id);ctx.navigate({...ctx.route,query},{replace:true});}},el('span',{class:'planning-item-top'},el('span',{class:'muted'},row.id),status(row.status)),el('strong',{},row.name),el('span',{class:'planning-item-meta'},`${row.status==='draft'?(row.proposed_count??0):(row.member_count??0)} 项${iteration&&row.status==='closed'?'冻结结果':row.status==='draft'?'提议需求':'需求'}`)));
  }
  const panel=el('section',{class:'planning-detail'},el('div',{class:'planning-title'},el('div',{},el('p',{class:'eyebrow'},selected.id),el('h2',{},selected.name)),status(selected.status)));
  panel.append(documentView(ctx,{table,id:selected.id,field:'description',title:'目标与说明'}));
  const count=selected.status==='draft'?(selected.proposed_count??0):(selected.member_count??0);
  const scope=iteration?{scope:'iteration',workset_id:selected.id,basis:'scope'}:{scope:'all',milestone_id:selected.id};
  panel.append(el('div',{class:'planning-scope'},el('div',{},el('h3',{},iteration&&selected.status==='closed'?'本轮冻结结果':'需求范围'),el('p',{class:'muted'},iteration?(selected.status==='closed'?`${count} 项结束记录；当前状态变化不改写本轮结果`:selected.status==='draft'?`${count} 项草案提议；尚未启动` :`${count} 项本轮范围，包含本轮已完成项`):`${count} 项当前或关闭归属记录；不代表阶段关闭时完整快照`)),button('查看需求 →',()=>ctx.navigate({page:'topics',projectID:ctx.route.projectID,query:scope}))));
  if(iteration&&selected.status==='active'){
    const progress=selected.progress||{};
    panel.append(el('div',{class:'progress-facts'},el('span',{},`当前排队 ${progress.current_queue_count??'—'}`),el('span',{},`本轮结束 ${progress.closed_in_round_count??'—'}`),button('查看全部参与（含移出）',()=>ctx.navigate({page:'topics',projectID:ctx.route.projectID,query:{...scope,basis:'history'}}))));
  }
  if(selected.closure){
    const closure=selected.closure;
    panel.append(el('section',{class:'closure-summary'},el('h3',{},'结项记录'),status(closure.outcome,resultNames[closure.outcome]||closure.outcome),closure.summary?el('p',{},closure.summary):null,closure.verification?el('p',{class:'muted'},`验证：${typeof closure.verification==='string'?closure.verification:'已保留结构化验证记录'}`):null,closure.closed_on?el('p',{class:'muted'},`结束日期 ${closure.closed_on}`):null));
    const describe=value=>value&&typeof value==='object'?(Array.isArray(value)?value.map(describe).join('；'):Object.entries(value).map(([key,item])=>`${key}：${describe(item)}`).join('；')):String(value??'—');
    const records=el('dl',{class:'evidence-list'});
    for(const [key,label] of [['evidence','结项证据'],['goal_acceptance','目标接受依据'],['delivery_record','交付记录']])if(closure[key]!=null)records.append(el('dt',{},label),el('dd',{},describe(closure[key])));
    if(closure.verification&&typeof closure.verification==='object')records.append(el('dt',{},'验证记录'),el('dd',{},describe(closure.verification)));
    if(records.childNodes.length)panel.append(el('details',{class:'closure-evidence'},el('summary',{},'查看结项依据'),records));
    if(iteration&&Array.isArray(closure.results)){
      const counts={};for(const row of closure.results)counts[row.disposition]=(counts[row.disposition]||0)+1;
      panel.append(el('div',{class:'result-counts'},Object.entries(counts).map(([key,value])=>el('span',{},`${resultNames[key]||key} ${value}`))));
    }
  }
  root.append(el('div',{class:'planning-layout'},list,panel),el('footer',{class:'page-footer'},`本地数据 · ${formatTime(view.read_at)} · revision ${view.revision}`));
  return {element:root};
}
