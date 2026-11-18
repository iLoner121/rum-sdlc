export async function render(ctx) {
  const {el,clear,icon,formatTime}=ctx.dom;const c=ctx.components;const state=ctx.state;
  state.query=state.query ?? ctx.route.query.get('q') ?? '';
  const rowsRegion=el('div');const notice=el('div');const feedback=el('div',{'aria-live':'polite'});
  const search=c.search({value:state.query||ctx.route.query.get('q')||'',placeholder:'搜索名称或位置',onInput:value=>{state.query=value;draw();}});
  const refresh=c.button('刷新',()=>load());
  const root=el('div',{},c.pageHeader('本机项目','已登记的管理工程'),el('div',{class:'toolbar'},search,refresh),notice,feedback,rowsRegion);
  let generation=0;
  function codeLabel(value){if(typeof value==='string')return ({unknown:'待核实','not-inspected':'待核实',unconfigured:'未配置',configured:'已关联',available:'可用'})[value]||value;return value?.label||value?.availability||'待核实';}
  async function copy(path){try{await navigator.clipboard.writeText(path);clear(feedback,el('div',{class:'notice notice-info',role:'status'},'已复制工程位置。'));}catch{clear(feedback,el('div',{class:'notice notice-info',role:'status'},'浏览器未允许复制，请从工程位置选取文字。'));}}
  function draw(){
    if(!state.data)return;
    const all=state.data.rows||[];const query=(state.query||'').trim().toLocaleLowerCase();
    const rows=query?all.filter(row=>[row.name,row.path,row.project_id,row.id].some(v=>String(v||'').toLocaleLowerCase().includes(query))):all;
    if(!all.length){clear(rowsRegion,c.empty('尚无已登记工程','当前设备没有可读取的工程登记。使用已有 CLI 配置工程后，再重新读取；也可浏览云端发现信息。',c.link('查看云端项目',{page:'cloud'},{class:'button'})));return;}
    if(!rows.length){clear(rowsRegion,c.empty('没有匹配的项目',`当前搜索：${state.query}`,c.button('清除搜索',()=>{state.query='';search.querySelector('input').value='';draw();})));return;}
    const body=el('tbody');
    for(const row of rows){
      const id=row.project_id||row.id;const available=row.local_available!==false && row.management_status!=='unavailable' && !row.error;
      const title=available?c.link(row.name||id,{page:'overview',projectID:id}):el('button',{type:'button',class:'text-link',onClick:()=>clear(feedback,c.errorNotice(row.error||{code:'path-unavailable',message:`工程位置 ${row.path||'未知'} 当前不可用。请通过已有 CLI 核对登记；浏览界面不会修复。`}))},row.name||id);
      body.append(el('tr',{},el('td',{},el('div',{class:'project-name'},el('span',{class:'project-avatar'},String(row.name||'P').slice(0,1)),el('div',{},el('strong',{},title),el('small',{},row.slug||row.description||id)))),el('td',{class:'path-cell'},el('button',{type:'button',class:'path-copy',title:'复制工程位置',onClick:()=>copy(row.path||'')},row.path||'未提供位置')),el('td',{},el('div',{class:'project-management',title:row.last_checked_at?`检查于 ${formatTime(row.last_checked_at)}`:''},c.status(available?'available':'unavailable',row.error?.code==='identity-invalid'?'身份异常':undefined),el('small',{},row.conflict?'同步冲突':row.sync_enabled===false?'未启用同步':({synced:'已同步','pending-push':'待推送',conflict:'同步冲突',local:'本地工程'})[row.management_status]||'同步待核实'))),el('td',{},codeLabel(row.code_status)),el('td',{class:'chevron-cell'},icon('chev'))));
    }
    const table=el('table',{class:'data-table project-table'},el('thead',{},el('tr',{},...['项目','工程位置','管理数据','源码',''].map(text=>el('th',{scope:'col'},text)))),body);
    clear(rowsRegion,el('div',{class:'table-scroll'},table),el('div',{class:'list-summary'},el('span',{},query?`${rows.length} 个匹配 / ${all.length} 个项目`:`${all.length} 个项目`),el('time',{title:state.readAt||''},state.readAt?`上次读取 ${formatTime(state.readAt)}`:'')));
  }
  async function load(){
    const call=++generation;refresh.disabled=true;clear(notice);if(!state.data)clear(rowsRegion,c.loading());
    try{const response=await ctx.api.get('projects');if(ctx.signal.aborted||call!==generation)return;state.data=response.data;state.readAt=response.read_at;for(const row of response.data.rows||[])ctx.setProject?.({id:row.project_id||row.id,name:row.name});draw();}
    catch(error){if(error.name==='AbortError'||ctx.signal.aborted||call!==generation)return;clear(notice,c.errorNotice(error,load,{staleAt:state.readAt}));if(!state.data)clear(rowsRegion);}
    finally{if(call===generation)refresh.disabled=false;}
  }
  if(state.data)draw();await load();return {element:root,dispose(){generation++;}};
}
