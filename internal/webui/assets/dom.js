// DOM construction is deliberately text-first. Repository content never becomes HTML.
export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class' || key === 'className') node.className = value;
    else if (key === 'dataset') Object.assign(node.dataset, value);
    else if (key.startsWith('on') && typeof value === 'function') node.addEventListener(key.slice(2).toLowerCase(), value);
    else if (key === 'value' || key === 'checked' || key === 'disabled' || key === 'selected') node[key] = value;
    else node.setAttribute(key, value === true ? '' : String(value));
  }
  append(node, children);
  return node;
}
export function append(node, children) {
  for (const child of children.flat(Infinity)) {
    if (child === undefined || child === null || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
}
export function clear(node, ...children) { node.replaceChildren(); append(node, children); return node; }
const paths = {
  grid: 'M2 2h6v6H2z M12 2h6v6h-6z M2 12h6v6H2z M12 12h6v6h-6z',
  cloud: 'M5 15h10a4 4 0 0 0 1-8 6 6 0 0 0-11-1 4.5 4.5 0 0 0 0 9',
  home: 'M2 9l8-7 8 7 M5 8v10h10V8 M8 18v-6h4v6',
  flag: 'M4 19V2 M4 3h12l-3 4 3 4H4', list: 'M6 4h12 M6 10h12 M6 16h12 M2 4h1 M2 10h1 M2 16h1',
  cycle: 'M17 7A8 8 0 0 0 3 5 M3 1v4h4 M3 13a8 8 0 0 0 14 2 M17 19v-4h-4',
  box: 'M2 6l8-4 8 4-8 4z M2 6v10l8 4 8-4V6 M10 10v10', search: 'M14 14l5 5 M15 8a7 7 0 1 1-14 0 7 7 0 0 1 14 0',
  chev: 'M7 5l5 5-5 5', refresh: 'M17 8a7 7 0 1 0 0 6 M17 2v6h-6', menu: 'M2 4h16 M2 10h16 M2 16h16',
  close: 'M4 4l12 12 M16 4L4 16', copy: 'M7 7h11v12H7z M3 14V2h11', arrow: 'M3 10h14 M12 5l5 5-5 5'
};
export function icon(name, className = '') {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  for (const [key, val] of Object.entries({viewBox:'0 0 20 20', fill:'none', stroke:'currentColor', 'stroke-width':'1.45', 'stroke-linecap':'round', 'stroke-linejoin':'round', 'aria-hidden':'true', class:`icon ${className}`})) svg.setAttribute(key, val);
  const path = document.createElementNS(svg.namespaceURI, 'path'); path.setAttribute('d', paths[name] || paths.list); svg.append(path); return svg;
}
export function formatTime(value) {
  if (!value) return '尚无读取记录';
  const date = new Date(value); return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString();
}
