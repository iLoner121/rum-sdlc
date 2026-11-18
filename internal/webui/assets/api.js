export class APIError extends Error {
  constructor(code, message, status = 0, details = null) { super(message); this.name = 'APIError'; this.code = code; this.status = status; this.details = details; }
}
export function createAPI(defaultSignal) {
  async function get(path, params = {}, options = {}) {
    const target = new URL(path.startsWith('/api/v1/') ? path : `/api/v1/${path.replace(/^\//, '')}`, location.origin);
    if (target.origin !== location.origin || !target.pathname.startsWith('/api/v1/')) throw new APIError('invalid-path', '请求地址不在只读接口范围内。');
    for (const [key, value] of (params instanceof URLSearchParams ? params.entries() : Object.entries(params || {}))) if (value !== undefined && value !== null && value !== '') target.searchParams.set(key, String(value));
    let response;
    try { response = await fetch(target, {method:'GET', credentials:'same-origin', cache:'no-store', signal: options.signal || defaultSignal, headers: {'Accept':'application/json'}}); }
    catch (error) { if (error.name === 'AbortError') throw error; throw new APIError('network', '无法连接本机服务，请检查服务是否仍在运行。'); }
    let envelope;
    try { envelope = await response.json(); } catch { throw new APIError('invalid-response', '本机服务返回了无法读取的响应。', response.status); }
    if (!response.ok || envelope.error) {
      const error = envelope.error || envelope;
      throw new APIError(error.code || (response.status === 409 ? 'stale' : 'unknown'), error.message || '暂时无法读取数据。', response.status, error.details);
    }
    if (envelope.schema_version !== 'gui-view/1') throw new APIError('unsupported', '界面与本机服务版本不兼容，请重新打开当前版本界面。');
    return envelope;
  }
  return { get, project: (id, suffix, params = {}, options = {}) => get(`projects/${encodeURIComponent(id)}/${suffix.replace(/^\//,'')}`, params, options) };
}
