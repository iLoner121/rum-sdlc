export async function render(ctx) {
  const {el,clear,formatTime}=ctx.dom;const c=ctx.components;const state=ctx.state;let generation=0;
  const body=el('div');const notice=el('div');const refresh=c.button('刷新',()=>load());const title=el('span',{},'项目概览');
  const root=el('div',{},c.pageHeader(title,'项目概览',refresh),notice,body);
  const nameOf=row=>row.name||row.title||row.short_id||row.id;
  const idOf=row=>row.id||row.milestone_id||row.workset_id;
  function draw(){
    const data=state.data;if(!data)return;const project=data.project||{};title.textContent=project.name||project.title||'项目概览';ctx.setProject?.(project);
    const counts=data.topic_counts||{};const entries=Object.entries(counts).filter(([,value])=>typeof value==='number');const total=typeof counts.total==='number'?counts.total:entries.filter(([key])=>key!=='total').reduce((sum,[,v])=>sum+v,0);
    const count=status=>typeof counts[status]==='number'?String(counts[status]):'—';
    const metrics=el('section',{class:'overview-metrics','aria-label':'需求状态统计'},...[[String(total),'全部需求',''],[count('active'),'进行中','active'],[count('review'),'待评审','review'],[count('closed'),'已关闭','closed']].map(([value,label,status])=>c.link(el('span',{},el('strong',{},value),el('span',{},label)),{page:'topics',projectID:ctx.route.projectID,query:status?{status}:{}},{class:'overview-metric'})));
    function collection(items,kind,label){
      const section=el('section',{},el('div',{class:'section-label'},label));
      if(!Array.isArray(items)||!items.length){section.append(el('p',{class:'quiet-empty'},kind==='milestones'?'当前没有活动里程碑':'当前没有活动迭代'),c.link(kind==='milestones'?'查看所有里程碑 →':'查看所有迭代 →',{page:kind,projectID:ctx.route.projectID},{class:'section-link'}));return section;}
      for(const row of items){
        const item=el('div',{class:'current-item'},el('div',{class:'current-title'},row.short_id?el('small',{},row.short_id):null,el('h2',{},nameOf(row))),c.status(row.status));
        const detail=row.summary||row.goal||row.description_summary;const memberCount=row.member_count ?? row.progress?.counts?.total ?? row.progress?.total;
        if(detail)item.append(el('p',{},detail));else if(memberCount!==undefined)item.append(el('p',{},`${memberCount} 项需求`));
        item.append(c.link(kind==='milestones'?'查看里程碑 →':'查看本轮 →',{page:kind,projectID:ctx.route.projectID,query:{selected:idOf(row)}},{class:'section-link'}));section.append(item);
      }
      return section;
    }
    const current=el('section',{class:'overview-current'},collection(data.milestones,'milestones','当前里程碑'),collection(data.iterations,'iterations','当前迭代'));
    const progress=el('section',{class:'progress-section'},el('h3',{},'需求进展'));
    const bar=el('div',{class:'status-bar',role:'img','aria-label':entries.filter(([key])=>key!=='total').map(([key,value])=>`${c.statusLabels[key]||key} ${value}`).join('，')||'尚无需求'});
    for(const [key,value] of entries)if(key!=='total'&&value>0)bar.append(el('span',{class:'status-segment','data-status':key,style:`flex:${value}`,title:`${c.statusLabels[key]||key} ${value}`}));
    const legend=el('div',{class:'progress-legend'},...entries.filter(([key,value])=>key!=='total'&&value>0).map(([key,value])=>c.link(c.status(key,`${c.statusLabels[key]||key} ${value}`),{page:'topics',projectID:ctx.route.projectID,query:{status:key}})));
    progress.append(bar,legend);if(total===0)progress.append(el('p',{class:'quiet-empty'},'尚无需求；当前阶段与迭代不会自动建立。'));
    const pool=data.pool||{};const poolCount=pool.count??pool.member_count;const poolSection=el('section',{class:'pool-summary'},el('h3',{},'需求池'),el('span',{},poolCount===undefined?'数量暂不可用':`${poolCount} 个候选`),c.link('查看需求池 →',{page:'pool',projectID:ctx.route.projectID},{class:'section-link'}));
    clear(body,metrics,current,progress,poolSection);
    const description=el('details',{class:'project-description',open:true},el('summary',{},'项目目标与说明'),c.documentView(ctx,{table:'project',id:project.id||ctx.route.projectID,field:'description',title:'项目说明'}));body.append(description);
  }
  async function load(){
    const call=++generation;refresh.disabled=true;clear(notice);if(!state.data)clear(body,c.loading());
    try{const response=await ctx.api.project(ctx.route.projectID,'overview');if(ctx.signal.aborted||call!==generation)return;state.data=response.data;state.readAt=response.read_at;draw();}
    catch(error){if(error.name==='AbortError'||ctx.signal.aborted||call!==generation)return;clear(notice,c.errorNotice(error,load,{staleAt:state.readAt}));if(!state.data)clear(body);}
    finally{if(call===generation)refresh.disabled=false;}
  }
  if(state.data)draw();await load();return {element:root,dispose(){generation++;}};
}
