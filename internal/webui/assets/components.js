import {el, icon, clear, formatTime} from './dom.js';
import {routeURL} from './router.js';
export const statusLabels = {planned:'待开始',active:'进行中',review:'待评审',ready:'待交付',closed:'已关闭',done:'已完成',draft:'草案',blocked:'受阻',cancelled:'已取消',deferred:'已转出',available:'可用',unavailable:'路径失效',unknown:'待核实',completed:'完成','carry-forward':'结转',backlog:'退回需求池',superseded:'被替代'};
export function status(value, label) {
  const color = ['closed','done','available','completed'].includes(value) ? 'success' : value === 'active' ? 'active' : ['review','blocked','unavailable','stale'].includes(value) ? 'warning' : 'muted';
  return el('span',{class:`status status-${color}`},el('span',{class:'status-dot'}),label || statusLabels[value] || value || '未知');
}
export function button(label, action, options = {}) {
  return el('button',{type:'button',class:`button ${options.primary ? 'button-primary' : ''} ${options.class || ''}`,disabled:options.disabled,onClick:action,title:options.title},options.icon ? icon(options.icon) : null,label);
}
export function link(label, route, options = {}) { return el('a',{href:routeURL(route),class:options.class || '',title:options.title},label); }
export function search({value = '',placeholder = '搜索',onInput} = {}) {
  const input = el('input',{type:'search',value,placeholder,'aria-label':placeholder,onInput:e=>onInput?.(e.target.value,e)});
  return el('label',{class:'search'},icon('search'),input);
}
export function empty(title, description, action) { return el('div',{class:'empty-state'},el('div',{class:'empty-symbol'},icon('box')),el('h2',{},title),el('p',{},description),action); }
const errorLabels = {authentication:'无法读取现有账号',permission:'当前账号没有访问权限',network:'连接暂时不可用','identity-invalid':'项目身份无法验证','path-unavailable':'工程位置不可用',conflict:'管理数据存在冲突',busy:'数据正在使用中',stale:'管理数据已更新',unsupported:'版本暂不兼容'};
export function errorNotice(error, retry, options = {}) {
  const message = typeof error === 'string' ? error : error?.message || '暂时无法读取数据。';
  return el('div',{class:`notice ${options.staleAt ? 'notice-stale' : 'notice-error'}`,role:'status'},el('div',{},el('strong',{},errorLabels[error?.code] || (options.staleAt ? '正在显示上次读取的数据' : '暂时无法读取')),el('p',{},message),options.staleAt ? el('small',{},`上次成功读取 ${formatTime(options.staleAt)}`) : null),retry ? button('重新读取',retry) : null);
}
export function loading(label = '正在读取…') { return el('div',{class:'loading-state',role:'status','aria-label':label},el('span',{class:'sr-only'},label),...[1,2,3].map(()=>el('div',{class:'skeleton'},el('span')))); }
export function tabs(items, selected, onChange) {
  return el('div',{class:'tabs',role:'tablist'},...items.map(item=> {
    const key = item.key ?? item.id ?? item.value; const label = item.label ?? item.name ?? key;
    return el('button',{type:'button',role:'tab','aria-selected':String(key === selected),class:key === selected ? 'selected' : '',onClick:()=>onChange?.(key)},label);
  }));
}
export function pager({offset = 0,limit = 30,total = 0,onPage} = {}) {
  const page = Math.floor(offset / limit); const pages = Math.ceil(total / limit);
  return el('div',{class:'pagination'},el('span',{},total ? `显示 ${offset+1}–${Math.min(offset+limit,total)}，共 ${total} 项` : '共 0 项'),el('div',{class:'pagination-actions'},button('上一页',()=>onPage?.(Math.max(0,offset-limit)),{disabled:offset===0}),el('span',{'aria-live':'polite'},`${pages ? page+1 : 0} / ${pages}`),button('下一页',()=>onPage?.(offset+limit),{disabled:offset+limit>=total})));
}
export function pageHeader(title, subtitle, actions) { return el('header',{class:'page-heading'},el('div',{},el('h1',{tabindex:'-1'},title),subtitle ? el('p',{},subtitle) : null),actions ? el('div',{class:'heading-actions'},actions) : null); }
export function topicTable(rows = [], options = {}) {
  const {projectID,onOpen,showPosition = false,historical = false} = options;
  const headings = [...(showPosition ? ['顺序'] : []),'编号','名称',historical ? '本轮结果' : '状态','里程碑','工作集合','Line',''];
  const table = el('table',{class:'data-table topic-table'},el('thead',{},el('tr',{},...headings.map(h=>el('th',{scope:'col'},h)))));
  const body = el('tbody');
  for (const raw of rows) {
    const row = raw.topic && typeof raw.topic === 'object' ? {...raw.topic,...raw,topic:undefined} : raw;
    const id = row.id || row.topic_id; const shortID = row.short_id || row.topic_short_id || id;
    const name = row.name || row.title || shortID;
    const relations = row.relations || {};
    const closedMemberships = row.status==='closed' ? row.closure?.memberships || {} : {};
    const closedTitle = row.status==='closed' ? '关闭时归属' : undefined;
    const membership=(key,fallback)=>Object.prototype.hasOwnProperty.call(closedMemberships,key)?closedMemberships[key]:fallback;
    const participation={current:'当前排队','completed-in-round':'本轮已结束',removed:'已移出'}[row.participation];
    const describe = value => typeof value === 'object' && value ? (value.short_id || value.name || value.id) : value;
    const target = {page:'detail',projectID,entityID:id,query: historical ? {history_workset:options.worksetID || '',disposition:row.disposition || ''} : {}};
    const anchor = link(name,target);
    if (onOpen) anchor.addEventListener('click',e=>{ if (!e.metaKey && !e.ctrlKey && !e.shiftKey && e.button===0) {e.preventDefault();onOpen(raw);} });
    body.append(el('tr',{},showPosition ? el('td',{class:'number-cell'},row.position ?? raw.position ?? '—') : null,el('td',{class:'number-cell'},shortID),el('td',{class:'title-cell'},anchor,participation?el('small',{class:'row-sublabel'},participation):null),el('td',{},status(historical ? row.disposition : row.status)),el('td',{title:historical?undefined:closedTitle},historical ? '当前详情中查看' : describe(membership('milestone_id',row.milestone || row.milestone_id || relations.milestone || relations.milestone_id)) || '—'),el('td',{title:historical?undefined:closedTitle},historical ? options.worksetID || '结束时记录' : describe(membership('workset_id',row.workset || row.workset_id || relations.workset || relations.workset_id)) || '—'),el('td',{title:historical?undefined:closedTitle},historical ? '当前详情中查看' : describe(membership('line_id',row.line || row.line_id || relations.line || relations.line_id)) || '—'),el('td',{class:'chevron-cell'},icon('chev'))));
  }
  table.append(body); return el('div',{class:'table-scroll'},table);
}
// Inline Markdown emits DOM nodes only. Raw HTML is always plain text.
function inline(text, options) {
  const fragment = document.createDocumentFragment();
  const pattern = /(!?\[([^\]]*)\]\(([^\s)]*)(?:\s+"[^"]*")?\)|`([^`]+)`|\*\*([^*]+)\*\*|__([^_]+)__|\*([^*\n]+)\*)/g;
  let last = 0; let match;
  while ((match = pattern.exec(text))) {
    fragment.append(document.createTextNode(text.slice(last,match.index)));
    if (match[4] !== undefined) fragment.append(el('code',{},match[4]));
    else if (match[5] !== undefined || match[6] !== undefined) fragment.append(el('strong',{},match[5] ?? match[6]));
    else if (match[7] !== undefined) fragment.append(el('em',{},match[7]));
    else if (match[1].startsWith('!')) fragment.append(el('span',{class:'image-placeholder'},`[图片：${match[2] || '未命名'} · 未自动加载]`));
    else {
      const label = match[2]; const href = match[3];
      if (/^https?:\/\//i.test(href)) {
        try { const url = new URL(href); fragment.append(el('a',{href:url.href,target:'_blank',rel:'noopener noreferrer'},label)); } catch { fragment.append(document.createTextNode(label)); }
      } else if (href.startsWith('#')) fragment.append(el('a',{href,onClick:e=>{e.preventDefault();options.onAnchor?.(href.slice(1));}},label));
      else if (href && !/^[a-z][a-z\d+.-]*:|^\/|\\/i.test(href) && options.onDocumentLink) fragment.append(el('button',{type:'button',class:'text-link',onClick:()=>options.onDocumentLink(href)},label));
      else fragment.append(el('span',{title:'此链接无法在只读界面中安全打开'},label));
    }
    last = pattern.lastIndex;
  }
  fragment.append(document.createTextNode(text.slice(last))); return fragment;
}
export function markdown(source = '', options = {}) {
  const root = el('article',{class:'markdown'}); const lines = String(source).replace(/\r\n?/g,'\n').split('\n'); let i = 0;
  const drawInline = text => inline(text, {...options,onAnchor:options.onAnchor || (id=> { const target = [...root.querySelectorAll('[id]')].find(n=>n.id===id); target?.scrollIntoView({block:'start'}); })});
  const isSpecial = line => /^\s*$|^\s*```|^#{1,6}\s|^\s*[-*+]\s|^\s*\d+[.)]\s|^\s*>|^\s*(?:-{3,}|\*{3,})\s*$/.test(line);
  const cells = line => line.trim().replace(/^\||\|$/g,'').split('|').map(v=>v.trim());
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) { i++; continue; }
    if (/^\s*```/.test(line)) {
      const language = line.trim().slice(3).trim(); const code = []; i++;
      while (i<lines.length && !/^\s*```/.test(lines[i])) code.push(lines[i++]);
      if (i<lines.length) i++;
      root.append(el('pre',{},el('code',{'data-language':language},code.join('\n')))); continue;
    }
    const heading = /^(#{1,6})\s+(.+?)\s*#*$/.exec(line);
    if (heading) { const id = heading[2].trim().toLowerCase().replace(/[^\p{L}\p{N}_-]+/gu,'-'); root.append(el(`h${heading[1].length}`,{id},drawInline(heading[2])));i++;continue; }
    if (/^\s*(?:-{3,}|\*{3,})\s*$/.test(line)) {root.append(el('hr'));i++;continue;}
    if (i+1<lines.length && line.includes('|') && /^\s*\|?\s*:?-{3,}/.test(lines[i+1])) {
      const table=el('table',{},el('thead',{},el('tr',{},...cells(line).map(c=>el('th',{},drawInline(c)))))); const body=el('tbody');i+=2;
      while(i<lines.length && lines[i].includes('|') && lines[i].trim()) body.append(el('tr',{},...cells(lines[i++]).map(c=>el('td',{},drawInline(c)))));
      table.append(body);root.append(el('div',{class:'table-scroll'},table));continue;
    }
    if (/^\s*[-*+]\s|^\s*\d+[.)]\s/.test(line)) {
      const ordered=/^\s*\d+[.)]\s/.test(line);const list=el(ordered?'ol':'ul');
      while(i<lines.length && (ordered ? /^\s*\d+[.)]\s/ : /^\s*[-*+]\s/).test(lines[i])) list.append(el('li',{},drawInline(lines[i++].replace(/^\s*(?:[-*+]|\d+[.)])\s+/,''))));
      root.append(list);continue;
    }
    if (/^\s*>/.test(line)) { const quote=[];while(i<lines.length && /^\s*>/.test(lines[i])) quote.push(lines[i++].replace(/^\s*>\s?/,''));root.append(el('blockquote',{},markdown(quote.join('\n'),options)));continue; }
    const paragraph=[line];i++;while(i<lines.length && !isSpecial(lines[i]) && !(i+1<lines.length && lines[i].includes('|') && /^\s*\|?\s*:?-{3,}/.test(lines[i+1]))) paragraph.push(lines[i++]);
    root.append(el('p',{},drawInline(paragraph.join('\n'))));
  }
  return root;
}
export function documentView(ctx, descriptor = {}) {
  const {table,id,field,title} = descriptor;
  const region=el('section',{class:'document-view'}); const content=el('div'); const breadcrumb=el('div',{class:'document-toolbar'}); region.append(breadcrumb,content);
  let generation=0; let current=null; const stack=[];
  async function load(request=null,push=true,anchor='') {
    const call=++generation; clear(content,loading('正在读取正文…'));
    try {
      const envelope=await ctx.api.project(ctx.route.projectID,request?'document-link':'documents',request || {table,id,field});
      if (ctx.signal.aborted || call!==generation) return;
      const data=envelope.data;
      if (push && current) stack.push(current);current={data,request};
      clear(breadcrumb,stack.length ? button('返回上级正文',()=> { const prev=stack.pop();load(prev.request,false); }) : null,el('span',{},data.title || title || field || '正文'),el('small',{},data.hash ? `版本 ${data.hash.slice(0,10)}` : ''));
      if (!data.exists) {clear(content,empty('尚未建立', '这份可选正文尚未建立。界面不会生成文件。'));return;}
      const article=markdown(data.content || '',{onDocumentLink:href=> {
        const hash=href.indexOf('#'); const path=hash<0?href:href.slice(0,hash); const fragment=hash<0?'':href.slice(hash+1);
        load(data.resource ? {resource:data.resource,href:path} : {table,id,field,href:path},true,fragment);
      }});
      clear(content,article,el('div',{class:'document-meta'},el('time',{datetime:data.read_at || envelope.read_at,title:data.read_at || envelope.read_at},`读取于 ${formatTime(data.read_at || envelope.read_at)}`)));
      if(anchor) { let decoded=anchor;try{decoded=decodeURIComponent(anchor);}catch{} requestAnimationFrame(()=>[...article.querySelectorAll('[id]')].find(n=>n.id===decoded)?.scrollIntoView({block:'start'})); }
    } catch(error) {if(error.name!=='AbortError' && !ctx.signal.aborted && call===generation) clear(content,errorNotice(error,()=>load(request,false,anchor)));}
  }
  load(); return region;
}
