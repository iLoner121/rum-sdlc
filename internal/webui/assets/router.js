const projectPages = new Set(['overview','milestones','iterations','topics','pool']);
export function parseRoute(hash = location.hash) {
  const source = hash.replace(/^#/, '').replace(/^\//, '') || 'local';
  const split = source.indexOf('?'); const path = split < 0 ? source : source.slice(0, split);
  const query = new URLSearchParams(split < 0 ? '' : source.slice(split + 1));
  let parts;
  try { parts = path.split('/').filter(Boolean).map(decodeURIComponent); } catch { return {page:'local',query:new URLSearchParams()}; }
  if (parts[0] === 'projects' && parts[1]) {
    const page = parts[2] || 'overview';
    if (page === 'topics' && parts[3]) return {page:'detail',projectID:parts[1],entityID:parts[3],query};
    if (projectPages.has(page)) return {page,projectID:parts[1],query};
  }
  if (parts[0] === 'local' || parts[0] === 'cloud') return {page:parts[0],query};
  return {page:'local',query:new URLSearchParams()};
}
export function routeURL(route) {
  if (typeof route === 'string') return route.startsWith('#') ? route : `#${route.startsWith('/') ? '' : '/'}${route}`;
  let path = route.page === 'cloud' ? '/cloud' : '/local';
  if (route.projectID) {
    const base = `/projects/${encodeURIComponent(route.projectID)}`;
    path = route.page === 'detail' ? `${base}/topics/${encodeURIComponent(route.entityID || '')}` : `${base}/${projectPages.has(route.page) ? route.page : 'overview'}`;
  }
  const query = route.query instanceof URLSearchParams ? route.query : new URLSearchParams(Object.entries(route.query || {}).filter(([,v])=>v !== undefined && v !== null && v !== ''));
  const suffix = query.toString(); return `#${path}${suffix ? `?${suffix}` : ''}`;
}
export function routeKey(route) { return routeURL(route); }
