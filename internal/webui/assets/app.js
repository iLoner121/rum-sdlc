import * as dom from './dom.js';
import * as components from './components.js';
import {createAPI} from './api.js';
import {parseRoute,routeURL,routeKey} from './router.js';
const {el,clear,icon,formatTime}=dom;
const labels={local:'本机项目',cloud:'云端项目',overview:'概览',milestones:'里程碑',topics:'需求',iterations:'迭代',pool:'需求池',detail:'需求详情'};
const modules={local:()=>import('./pages/local.js'),cloud:()=>import('./pages/cloud.js'),overview:()=>import('./pages/overview.js'),milestones:()=>import('./pages/milestones.js'),topics:()=>import('./pages/topics.js'),iterations:()=>import('./pages/iterations.js'),pool:()=>import('./pages/pool.js'),detail:()=>import('./pages/detail.js')};
const sessions=new Map();const projects=new Map();let epoch=0,controller,active,dispose,device,cancelScrollRestore;
try{history.scrollRestoration='manual';}catch{}
const app=document.getElementById('app');
const projectLabel=el('div',{class:'project-identity'});
const projectNav=el('nav',{'aria-label':'项目导航'});
const workspaceNav=el('nav',{'aria-label':'工作空间'});
const topTitle=el('span',{class:'topbar-title'},'工作空间');
const serviceLabel=el('span',{class:'service-indicator'},'本机服务');
const footerTime=el('time',{},'');
const footerState=el('span',{class:'footer-state'},'本地数据');
const main=el('main',{id:'main-content',class:'page-content','aria-live':'polite'});
const shell=el('div',{class:'app-shell'});
function closeNav(){shell.classList.remove('nav-open');menu.setAttribute('aria-expanded','false');}
const menu=components.button('',()=>{const open=shell.classList.toggle('nav-open');menu.setAttribute('aria-expanded',String(open));},{icon:'menu',class:'menu-toggle'});menu.setAttribute('aria-label','切换导航');menu.setAttribute('aria-expanded','false');
const sidebar=el('aside',{class:'sidebar'},el('a',{class:'brand',href:'#/local'},el('span',{class:'brand-mark'},'R'),el('span',{},'RumSDLC')),el('div',{class:'nav-label'},'工作空间'),workspaceNav,el('hr',{class:'sidebar-rule'}),el('div',{class:'nav-label'},'当前项目'),projectLabel,projectNav,el('div',{class:'sidebar-bottom'},serviceLabel,el('button',{type:'button',class:'sidebar-about',onClick:showAbout},'账号信息  /  关于')));
shell.append(el('div',{class:'nav-scrim',onClick:closeNav}),sidebar,el('div',{class:'main-shell'},el('header',{class:'topbar'},menu,topTitle,el('span',{class:'topbar-mode'},'只读浏览')),main,el('footer',{class:'footer'},footerState,footerTime)));
clear(app,shell);
function remember(){if(active){const state=sessions.get(routeKey(active));if(state)state.scrollY=window.scrollY;}}
function navigate(next,options={}) {
  remember();let url=routeURL(next);const parsed=parseRoute(url);if(parsed.page==='detail' && active && active.page!=='detail' && !parsed.query.has('from')){parsed.query.set('from',routeURL(active));url=routeURL(parsed);}const key=routeKey(parsed);
  if(parsed.page==='detail' && active?.page!=='detail') {const state=sessions.get(key)||{};state.returnTo=routeURL(active);sessions.set(key,state);}
  if(options.replace){history.replaceState(null,'',url);render();}
  else if(location.hash!==url)location.hash=url;
  else render();
}
function setProject(project){if(!project)return;const id=project.id||project.project_id;if(id){projects.set(id,project);if(active?.projectID===id)paintNavigation(active);}}
function navItem(page,label,name,route,selected){return el('a',{class:`nav-item${selected?' selected':''}`,href:routeURL(route),'aria-current':selected?'page':null},icon(name),el('span',{},label));}
function paintNavigation(route){
  clear(workspaceNav,navItem('local','本机项目','grid',{page:'local'},route.page==='local'),navItem('cloud','云端项目','cloud',{page:'cloud'},route.page==='cloud'));
  const project=projects.get(route.projectID);const selected=route.projectID;
  clear(projectLabel,selected?el('strong',{},project?.name||project?.title||selected):el('strong',{},'尚未选择项目'),el('small',{},selected?'本机 · 管理工程浏览':'从本机项目开始浏览'));
  clear(projectNav,...(selected?[['overview','概览','home'],['milestones','里程碑','flag'],['iterations','迭代','cycle'],['pool','需求池','box'],['topics','需求','list']].map(([page,label,name])=>navItem(page,label,name,{page,projectID:selected},route.page===page || route.page==='detail'&&page==='topics')):[]));
  topTitle.textContent=selected?(project?.name||project?.title||selected):'工作空间';document.title=`${labels[route.page]||'RumSDLC'} · ${project?.name||'RumSDLC'}`;
}
// Restore after asynchronous document layout, with a short lifetime and user intent taking priority.
function scrollRestoration(target,signal,call){
  let stopped=false,started=false,observer,timer,frame,expectedY=window.scrollY;
  const scrollKeys=new Set(['ArrowUp','ArrowDown','PageUp','PageDown','Home','End',' ']);
  function stop(){
    if(stopped)return;stopped=true;observer?.disconnect();clearTimeout(timer);cancelAnimationFrame(frame);
    for(const name of ['wheel','touchstart','pointerdown'])window.removeEventListener(name,onIntent,true);
    window.removeEventListener('keydown',onKey,true);window.removeEventListener('scroll',onScroll,true);signal.removeEventListener('abort',stop);
  }
  function onIntent(){stop();}
  function onKey(event){const editable=event.target?.closest?.('input,textarea,select,[contenteditable="true"]');if(!editable&&scrollKeys.has(event.key))stop();}
  function onScroll(){if(started&&Math.abs(window.scrollY-expectedY)>2)stop();}
  function attempt(){
    if(stopped||signal.aborted||call!==epoch){stop();return;}
    const max=Math.max(0,document.documentElement.scrollHeight-window.innerHeight);
    expectedY=Math.min(target,max);
    if(Math.abs(window.scrollY-expectedY)>1)window.scrollTo(0,expectedY);
    if(Math.abs(window.scrollY-target)<=2)stop();
  }
  for(const name of ['wheel','touchstart','pointerdown'])window.addEventListener(name,onIntent,{capture:true,passive:true});
  window.addEventListener('pageshow',event=>{if(event.persisted)render();});
window.addEventListener('keydown',onKey,true);window.addEventListener('scroll',onScroll,{capture:true,passive:true});signal.addEventListener('abort',stop,{once:true});
  return {stop,start(){
    if(stopped)return;started=true;timer=setTimeout(stop,3000);
    if(typeof ResizeObserver==='function'){observer=new ResizeObserver(()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(attempt);});observer.observe(main);observer.observe(document.documentElement);}
    frame=requestAnimationFrame(attempt);
  }};
}
async function render(){
  remember();cancelScrollRestore?.();cancelScrollRestore=null;controller?.abort();dispose?.();dispose=null;controller=new AbortController();const signal=controller.signal;const call=++epoch;
  const route=parseRoute();active=route;const key=routeKey(route);if(!sessions.has(key))sessions.set(key,{});const state=sessions.get(key);const restoration=scrollRestoration(Math.max(0,Number(state.scrollY)||0),signal,call);cancelScrollRestore=restoration.stop;closeNav();paintNavigation(route);
  clear(main,components.pageHeader(labels[route.page]||'RumSDLC'),components.loading());main.setAttribute('aria-busy','true');
  const baseAPI=createAPI(signal);
  const api={get:async(...args)=> {const response=await baseAPI.get(...args);if(!signal.aborted&&call===epoch){footerTime.dateTime=response.read_at||'';footerTime.textContent=response.read_at?`读取于 ${formatTime(response.read_at)}`:'';footerTime.title=response.read_at||'';serviceLabel.textContent='本机服务';if(response.data?.project)setProject(response.data.project);}return response;},project:(id,suffix,params={},options={})=>api.get(`projects/${encodeURIComponent(id)}/${suffix.replace(/^\//,'')}`,params,options)};
  const ctx={route,api,navigate,signal,epoch:call,state,components,dom,setProject,returnTo:state.returnTo};
  let page;
  try{
    const module=await modules[route.page]();
    if(signal.aborted||call!==epoch)return;
    page=await module.render(ctx);
    if(signal.aborted||call!==epoch){page?.dispose?.();return;}
    clear(main,page.element||page);dispose=page.dispose;
  }catch(error){
    if(error.name==='AbortError'||signal.aborted||call!==epoch)return;
    // A page can be added independently without breaking already available routes.
    if(error instanceof TypeError && /module|import|fetch|script/i.test(error.message)) clear(main,components.pageHeader(labels[route.page]||'页面'),components.empty('此页面正在接入','共享浏览框架已就绪。可以返回项目概览或选择其他页面。',components.link('返回项目概览',{page:route.projectID?'overview':'local',projectID:route.projectID},{class:'button'})));
    else clear(main,components.pageHeader(labels[route.page]||'页面'),components.errorNotice(error,()=>render()));
  }finally{
    if(!signal.aborted&&call===epoch){main.removeAttribute('aria-busy');restoration.start();}
  }
}
async function showAbout(){
  const dialog=el('dialog',{class:'about-dialog','aria-labelledby':'about-title'},el('h2',{id:'about-title'},'RumSDLC'),el('p',{},'本机浏览界面与 CLI 使用同一套程序、同一版本。页面仅重新读取信息，不执行登记、导入、同步或配置。'));
  const body=el('div',{},components.loading());dialog.append(body,el('div',{class:'dialog-actions'},components.button('关闭',()=>dialog.close())));document.body.append(dialog);dialog.addEventListener('close',()=>dialog.remove(),{once:true});dialog.showModal();
  try{const response=await createAPI().get('device');device=response.data;clear(body,el('dl',{},el('dt',{},'程序版本'),el('dd',{},device.version||'未知'),el('dt',{},'核心协议'),el('dd',{},device.core_version||'未知'),el('dt',{},'当前账号'),el('dd',{},typeof device.account==='string'?device.account:device.account?.login||'尚无可用账号'),el('dt',{},'服务'),el('dd',{},device.service==='foreground'?'前台本机服务':device.service||'本机服务')));}
  catch(error){clear(body,components.errorNotice(error));}
}
document.addEventListener('click',event=>{const anchor=event.target.closest?.('a[href^="#/"]');if(!anchor||event.defaultPrevented||event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;event.preventDefault();navigate(anchor.getAttribute('href'));});
window.addEventListener('hashchange',render);window.addEventListener('pagehide',()=>{remember();cancelScrollRestore?.();controller?.abort();dispose?.();});
window.addEventListener('pageshow',event=>{if(event.persisted)render();});
window.addEventListener('keydown',event=>{if(event.key==='Escape')closeNav();});
render();
