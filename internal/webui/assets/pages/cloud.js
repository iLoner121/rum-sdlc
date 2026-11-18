// Discovery is session memory only. There are no import, sync or account writes.
export async function render(ctx) {
  const {el} = ctx.dom;
  const c = ctx.components;
  const state = ctx.state;
  state.query ??= ctx.route.query.get('q') || '';
  let generation = 0;
  let disposed = false;
  let pending = null;
  let error = null;
  let loading = false;
  const notices = el('div');
  const account = el('span', {class:'muted'});
  const timestamp = el('span', {class:'muted'});
  const results = el('div');
  const refresh = c.button('刷新', () => load(), {icon:'refresh'});
  const element = el('section', {class:'cloud-page'},
    c.pageHeader('云端项目', 'GitHub 上可访问的管理工程'),
    notices,
    el('div', {class:'list-toolbar'},
      c.search({value:state.query, placeholder:'搜索项目或仓库', onInput:value => {state.query = value; drawResults();}}),
      el('div', {class:'toolbar-actions'}, account, refresh)),
    results,
    el('div', {class:'list-footer'}, timestamp));

  const manifestLabels = {valid:'身份已验证', missing:'缺少身份文件', invalid:'身份无效', unavailable:'身份读取失败'};
  const localLabels = {available:'已在本机', 'not-imported':'未在本机', 'path-unavailable':'本机路径不可用', 'identity-conflict':'身份冲突'};
  const codeLabels = {git:'已关联 Git 仓库', directory:'普通代码目录', unconfigured:'未关联代码仓库', unknown:'关联未知'};
  const time = value => {
    if (!value) return '尚无读取记录';
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? String(value) : parsed.toLocaleString();
  };
  function drawResults() {
    const data = state.discovery?.data;
    results.replaceChildren();
    if (!data) {
      if (loading) results.append(c.loading('正在只读发现项目…'));
      else if (!error) results.append(c.empty('尚未读取云端项目', '刷新将使用已有凭据，只读查询可访问的管理仓库。'));
      return;
    }
    const all = Array.isArray(data.rows) ? data.rows : [];
    const query = state.query.trim().toLocaleLowerCase();
    const rows = all.filter(row => [row.display_name,row.name,row.full_name,row.project_id].some(value => String(value || '').toLocaleLowerCase().includes(query)));
    if (!all.length) {
      results.append(c.empty('没有发现管理项目', '本次查询成功，当前凭据可访问范围内没有名称以 RumSDLC_ 开头的仓库。'));
    } else if (!rows.length) {
      results.append(c.empty('没有匹配的项目', `当前搜索匹配 0 项，共有 ${all.length} 个发现结果。`));
    } else {
      const body = el('tbody');
      for (const row of rows) {
        const local = row.local || {};
        const code = row.code || {};
        const verified = row.manifest_status === 'valid';
        const project = el('div', {class:'project-identity'}, el('strong',{},row.display_name || row.name || '未命名仓库'),
          el('div',{},c.status(verified ? 'available' : 'blocked', manifestLabels[row.manifest_status] || '身份待核实')),
          row.project_id ? el('small',{class:'muted'},row.project_id) : null,
          row.error ? el('small',{class:'muted',title:row.error.code || ''},row.error.message || '读取失败') : null);
        const repository = /^https:\/\/github\.com\/[A-Za-z0-9][A-Za-z0-9-]*\/[A-Za-z0-9_.-]+$/.test(row.url || '')
          ? el('a',{href:row.url,target:'_blank',rel:'noopener noreferrer'},row.full_name || row.name)
          : el('span',{},row.full_name || row.name || '—');
        const localState = el('div',{},localLabels[local.status] || '本机状态未知',
          local.reason ? el('small',{class:'muted'},local.reason) : null);
        const sourceState = el('div',{},codeLabels[code.association] || '关联未知',
          el('small',{class:'muted',title:`来源：${code.source || '未知'}；读取于 ${time(code.read_at)}`},'本机源码待核实'));
        const open = verified && local.status === 'available' && local.project_id
          ? c.link('打开本机项目',{page:'overview',projectID:local.project_id})
          : el('span',{class:'muted'},verified ? '仅浏览发现信息' : '不可关联');
        body.append(el('tr',{},el('td',{class:'title-cell'},project),el('td',{},repository),el('td',{},localState),el('td',{},sourceState),el('td',{},open)));
      }
      results.append(el('div',{class:'table-scroll'},el('table',{class:'data-table cloud-table'},
        el('thead',{},el('tr',{},...['项目','管理仓库','本机状态','源码',''].map(title=>el('th',{scope:'col'},title)))),body)));
    }
    results.append(el('p',{class:'muted'},`${rows.length} / ${all.length} 个发现结果 · ${data.verified_total ?? all.filter(row=>row.manifest_status==='valid').length} 个身份已验证`));
  }
  function drawMeta() {
    const data = state.discovery?.data;
    account.textContent = data?.account?.login ? `账号：${data.account.login}` : '账号尚未确认';
    account.title = data?.scope || '使用已有认证上下文；界面不修改账号或凭据';
    timestamp.textContent = state.discovery ? `${error ? '上次成功读取' : '读取于'} ${time(state.discovery.read_at)}${loading ? ' · 正在刷新…' : ''}` : '';
    timestamp.setAttribute('title', state.discovery?.read_at || '');
    refresh.disabled = loading;
    notices.replaceChildren();
    if (error) notices.append(c.errorNotice(error,()=>load(),{staleAt:state.discovery?.read_at}));
    if (data) notices.append(el('p',{class:'muted'},data.scope || '结果仅代表当前凭据可访问的范围。'));
  }
  async function load() {
    pending?.abort();
    const controller = new AbortController();
    pending = controller;
    const current = ++generation;
    loading = true;
    error = null;
    drawMeta();
    drawResults();
    try {
      const envelope = await ctx.api.get('remote-projects', {}, {signal:controller.signal});
      if (disposed || ctx.signal.aborted || current !== generation) return;
      // A changed account replaces the entire old discovery atomically.
      state.discovery = envelope;
    } catch (failure) {
      if (disposed || ctx.signal.aborted || current !== generation || failure.name === 'AbortError') return;
      error = failure;
      if (failure.code === 'account-changed') delete state.discovery;
    } finally {
      if (!disposed && !ctx.signal.aborted && current === generation) {
        loading = false;
        drawMeta();
        drawResults();
      }
    }
  }
  const stop = () => { disposed = true; generation++; pending?.abort(); };
  ctx.signal.addEventListener('abort', stop, {once:true});
  load();
  return {element, dispose:stop};
}
