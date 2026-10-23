// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/*
 * Vista previa de las plantillas de Pages para REVISAR su diseño al escribirlas.
 *
 * Por qué existe: el renderizador de `@sytue/pages-render` aún no aplica `layout`, `size`, `style`,
 * `breakpoints` ni el tema (solo pinta el HTML y unos pocos estilos comunes), así que una plantilla
 * se vería plana. Esto pinta el IR siguiendo la tabla de §4.2 de PAGES-PLAN.md (flex, tamaños fill/hug,
 * columnas, tokens, estilos de texto, puntos de corte 1024 y 640) para poder mirar la composición.
 * Es una herramienta de desarrollo: no se publica ni sustituye al renderizador. Cuando éste cumpla
 * §4.2, bastará con fotografiar su salida y este fichero se puede retirar.
 *
 *   bun vista-previa.mjs <directorio de plantillas> <fuentes> <salida> [clave ...]
 */
import { mkdirSync, readdirSync, readFileSync, writeFileSync, copyFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';

const [dirPl, dirFuentes, salida, ...pedidas] = process.argv.slice(2);
mkdirSync(salida, { recursive: true });
const PUNTOS = { desktop: '', tablet: '(max-width:1023.98px)', mobile: '(max-width:639.98px)' };
const LANG = 'es';
const esc = (s) => String(s).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
const tx = (t) => (t && (t[LANG] ?? t.es ?? t.en)) ?? '';
const rico = (c) => ((c && (c[LANG] ?? c.es ?? c.en)) ?? []).map((r) => esc(r.text)).join('');
const SERIF = ['playfair-display', 'lora', 'source-serif-4', 'noto-serif'];
const val = (v) => (typeof v === 'string' && v.startsWith('$') ? `var(--c-${v.slice(1)})` : v);
const radio = (r) => (typeof r === 'string' ? `var(--r-${r.slice(1)})` : `${r}px`);

function fuentesCss(fuentes) {
  let css = '';
  for (const fam of new Set(fuentes)) {
    const f = join(dirFuentes, fam, 'latin.woff2');
    if (!existsSync(f)) continue;
    mkdirSync(join(salida, 'fuentes', fam), { recursive: true });
    copyFileSync(f, join(salida, 'fuentes', fam, 'latin.woff2'));
    css += `@font-face{font-family:"${fam}";src:url(/fuentes/${fam}/latin.woff2) format("woff2");font-weight:100 900;font-display:swap}`;
  }
  return css;
}

function cssTema(ir) {
  const t = ir.theme;
  const colores = Object.keys(t.colors).sort().map((k) => `--c-${k}:${t.colors[k]}`).join(';');
  const fam = (f) => `"${f}",${SERIF.includes(f) ? 'Georgia,serif' : 'system-ui,sans-serif'}`;
  let css = `:root{${colores};--r-sm:${t.radius.sm}px;--r-md:${t.radius.md}px;--r-lg:${t.radius.lg}px;--cont:${t.container}px}`;
  css += `*{box-sizing:border-box}body{margin:0;background:var(--c-background);color:var(--c-text);font-family:${fam(t.fonts.body)}}`;
  css += 'img,iframe,table{max-width:100%}a{color:inherit}ul,ol{list-style:none;margin:0;padding:0}h1,h2,h3,h4,h5,h6,p,blockquote{margin:0}';
  for (const [clave, e] of Object.entries(t.text_styles)) {
    const f = fam(e.font === 'heading' ? t.fonts.heading : t.fonts.body);
    const base = `font-family:${f};line-height:${e.line_height};font-weight:${e.weight};letter-spacing:${e.letter_spacing}em${e.transform ? `;text-transform:${e.transform}` : ''}`;
    css += `.t-${clave}{${base};font-size:${e.size[0]}px}`;
    css += `@media${PUNTOS.tablet}{.t-${clave}{font-size:${e.size[1]}px}}@media${PUNTOS.mobile}{.t-${clave}{font-size:${e.size[2]}px}}`;
  }
  return css;
}

const PRESET_TAG = { h1: 'h1', h2: 'h2', h3: 'h3', h4: 'h4', h5: 'h5', h6: 'h6', p: 'body', span: 'body', blockquote: 'body' };
const CSS_ALIGN = { start: 'flex-start', center: 'center', end: 'flex-end', baseline: 'baseline' };
const CSS_JUST = { start: 'flex-start', center: 'center', end: 'flex-end', 'space-between': 'space-between', 'space-around': 'space-around', 'space-evenly': 'space-evenly' };

function efectivo(n, punto) {
  // base + deltas hasta el punto
  const out = { size: { ...(n.size ?? {}) }, layout: n.layout ? { ...n.layout } : null, style: { ...(n.style ?? {}) }, hidden: !!n.hidden };
  for (const p of punto === 'desktop' ? [] : punto === 'tablet' ? ['tablet'] : ['tablet', 'mobile']) {
    const d = n.breakpoints?.[p];
    if (!d) continue;
    if (d.hidden !== undefined) out.hidden = d.hidden;
    if (d.size) for (const [k, v] of Object.entries(d.size)) { if (v === null) delete out.size[k]; else out.size[k] = v; }
    if (d.layout && out.layout) Object.assign(out.layout, d.layout);
    if (d.columns !== undefined && out.layout) out.layout.columns = d.columns;
    if (d.style) Object.assign(out.style, d.style);
  }
  return out;
}

const DEFECTO = { button: { width: 'hug', height: 'hug' }, menu: { width: 'hug', height: 'hug' }, cart_button: { width: 'hug', height: 'hug' }, icon: { width: 24, height: 24 } };

function reglas(n, e, padre) {
  const r = [];
  const tam = { ...(DEFECTO[n.type] ?? { width: 'fill', height: 'hug' }), ...e.size };
  const dirPadre = padre?.layout?.direction ?? 'column';
  const gapPadre = padre?.layout?.gap ?? 0;
  const col = padre?.layout?.wrap && padre.layout.columns ? padre.layout.columns : 0;
  const principal = dirPadre === 'row' ? 'width' : 'height';
  const cruzado = dirPadre === 'row' ? 'height' : 'width';
  if (padre) {
    if (col) {
      r.push(`flex:0 0 calc((100% - ${(col - 1) * gapPadre}px)/${col})`, 'min-width:0');
    } else {
      const m = tam[principal];
      if (m === 'fill') {
        r.push('flex:1 1 0%');
        r.push(dirPadre === 'row' ? (e.size.min_width == null ? 'min-width:0' : '') : (e.size.min_height == null ? 'min-height:0' : ''));
      } else if (m === 'hug' || m === undefined) r.push('flex:0 0 auto');
      else r.push(`flex:0 0 ${m}px`, `${principal}:${m}px`, 'max-width:100%');
      const c = tam[cruzado];
      if (c === 'fill') r.push('align-self:stretch');
      else if (typeof c === 'number') r.push(`${cruzado}:${c}px`);
    }
  }
  if (e.size.min_width != null) r.push(`min-width:${e.size.min_width}px`);
  if (e.size.max_width != null) r.push(`max-width:${e.size.max_width}px`);
  if (e.size.min_height != null) r.push(`min-height:${e.size.min_height}px`);
  if (e.size.max_height != null) r.push(`max-height:${e.size.max_height}px`);
  if (e.size.aspect_ratio != null) r.push(`aspect-ratio:${e.size.aspect_ratio}`);
  if (n.type === 'image' && !padre) r.push('width:100%');
  if (n.type === 'image' && tam.width === 'fill') r.push('width:100%', 'height:auto');
  if ((n.type === 'frame' || n.type === 'form') && e.layout) {
    const l = e.layout;
    r.push('display:flex', `flex-direction:${l.direction}`, `gap:${l.gap}px`, `padding:${l.padding.map((x) => x + 'px').join(' ')}`, `align-items:${CSS_ALIGN[l.align]}`, `justify-content:${CSS_JUST[l.justify]}`, `flex-wrap:${l.wrap ? 'wrap' : 'nowrap'}`);
  }
  const s = e.style;
  if (s.background) r.push(`background:${val(s.background)}`);
  if (s.color) r.push(`color:${val(s.color)}`);
  if (s.border) r.push(`border:${s.border.width}px ${s.border.style} ${val(s.border.color)}`);
  if (s.radius !== undefined) r.push(`border-radius:${radio(s.radius)}`);
  if (s.overflow) r.push(`overflow:${s.overflow}`);
  if (s.shadow && s.shadow !== 'none') r.push('box-shadow:0 2px 10px rgba(0,0,0,.12)');
  if (s.text) {
    const t = s.text;
    if (t.align) r.push(`text-align:${t.align === 'start' ? 'left' : t.align}`);
    if (t.transform) r.push(`text-transform:${t.transform}`);
    if (t.weight) r.push(`font-weight:${t.weight}`);
    if (t.size) r.push(`font-size:${t.size}px`);
    if (t.italic) r.push('font-style:italic');
  }
  if (e.hidden) r.push('display:none');
  return r.filter(Boolean);
}

function reglasSeccion(n, e) {
  const r = [];
  const l = e.layout;
  const s = e.style;
  if (s.background) r.push(`background:${val(s.background)}`);
  if (s.color) r.push(`color:${val(s.color)}`);
  if (s.border) r.push(`border-bottom:${s.border.width}px ${s.border.style} ${val(s.border.color)}`);
  r.push(`padding:${l.padding.map((x) => x + 'px').join(' ')}`);
  if (e.hidden) r.push('display:none');
  const dentro = ['display:flex', `flex-direction:${l.direction}`, `gap:${l.gap}px`, `align-items:${CSS_ALIGN[l.align]}`, `justify-content:${CSS_JUST[l.justify]}`, `flex-wrap:${l.wrap ? 'wrap' : 'nowrap'}`, 'max-width:var(--cont)', 'margin-inline:auto', 'width:100%'];
  return [r, dentro];
}

const acum = { css: {}, n: 0 };
const clase = (id) => `n${id}`;

function emite(n, padre) {
  const base = padre ? null : null;
  void base;
  const por = {};
  for (const punto of ['desktop', 'tablet', 'mobile']) {
    const e = efectivo(n, punto);
    let r;
    if (n.type === 'section') {
      const [fuera, dentro] = reglasSeccion(n, e);
      por[punto] = { fuera, dentro };
    } else {
      r = reglas(n, e, padre ? { layout: efectivo(padre, punto).layout } : null);
      por[punto] = r;
    }
  }
  const c = clase(n.id);
  const sel = n.type === 'section' ? [`.${c}`, `.${c}>.sp-c`] : [`.${c}`];
  let previo = null;
  for (const punto of ['desktop', 'tablet', 'mobile']) {
    const actual = n.type === 'section' ? [por[punto].fuera.join(';'), por[punto].dentro.join(';')] : [por[punto].join(';')];
    const cambia = !previo || actual.some((x, i) => x !== previo[i]);
    if (cambia) {
      const reglasTxt = actual.map((x, i) => `${sel[i]}{${x}}`).join('');
      acum.css[punto] = (acum.css[punto] ?? '') + reglasTxt;
    }
    previo = actual;
  }
  return c;
}

const ICONOS = {
  phone: 'M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2z',
  mail: 'M3 6h18v12H3zM3 7l9 7 9-7',
  'map-pin': 'M12 21s7-6.2 7-11a7 7 0 0 0-14 0c0 4.8 7 11 7 11zM12 12a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z',
  clock: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2',
  whatsapp: 'M4 20l1.3-4.2A8 8 0 1 1 8.4 18.8L4 20z',
};
const icono = (nombre, tam = 20) => `<svg width="${tam}" height="${tam}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="${ICONOS[nombre] ?? 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z'}"/></svg>`;

function enlace(e, ir) {
  if (!e) return '#';
  if (e.url) return e.url;
  if (e.page) {
    const p = ir.pages.find((x) => x.id === e.page);
    const slug = p ? tx(p.slug) : '';
    return `${slug ? slug + '/' : ''}index.html${e.anchor ? '#' + e.anchor : ''}`.replace(/^/, '/__BASE__/');
  }
  if (e.anchor) return `#${e.anchor}`;
  return '#';
}

function placeholderSvg(tipo, ratio) {
  const rr = ratio ?? { photo: 1.5, portrait: 0.8, product: 1, landscape: 1.78, logo: 3 }[tipo] ?? 1.5;
  const w = 600;
  const h = Math.round(w / rr);
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}"><rect width="${w}" height="${h}" fill="#d9dbdd"/><path d="M0 ${h}L${w * 0.35} ${h * 0.45}L${w * 0.55} ${h * 0.7}L${w * 0.7} ${h * 0.55}L${w} ${h}z" fill="#c3c6c9"/><circle cx="${w * 0.78}" cy="${h * 0.25}" r="${h * 0.09}" fill="#c3c6c9"/></svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

function pinta(n, ir, padre) {
  const c = emite(n, padre);
  const cl = (extra = '') => `class="${c}${extra ? ' ' + extra : ''}"`;
  const hijos = () => (n.children ?? []).map((h) => pinta(h, ir, n)).join('');
  switch (n.type) {
    case 'section': {
      const tag = n.kind === 'header' ? 'header' : n.kind === 'footer' ? 'footer' : 'section';
      return `<${tag} ${cl()}${n.anchor ? ` id="${n.anchor}"` : ''}><div class="sp-c">${hijos()}</div></${tag}>`;
    }
    case 'frame': {
      const inner = `<${n.tag} ${cl()}>${hijos()}</${n.tag}>`;
      return n.link ? `<a href="${esc(enlace(n.link, ir))}" style="display:contents">${inner}</a>` : inner;
    }
    case 'text': {
      const preset = n.style?.text?.preset ?? PRESET_TAG[n.tag];
      return `<${n.tag} ${cl(`t-${preset}`)}>${rico(n.content)}</${n.tag}>`;
    }
    case 'image': {
      const r = n.size?.aspect_ratio;
      return `<img ${cl()} src="${placeholderSvg(n.src.placeholder, r)}" alt="${esc(tx(n.alt))}" style="object-fit:${n.fit}">`;
    }
    case 'button': {
      const v = n.variant;
      return `<a ${cl(`bt bt-${v} t-button`)} href="${esc(enlace(n.action, ir))}">${n.icon ? icono(n.icon, 18) : ''}${esc(tx(n.label))}</a>`;
    }
    case 'icon':
      return `<span ${cl()} role="img" aria-label="${esc(tx(n.label) || n.name)}">${icono(n.name, 22)}</span>`;
    case 'menu': {
      const li = n.items.map((i) => `<li><a href="${esc(enlace(i.link, ir))}">${esc(tx(i.label))}</a></li>`).join('');
      return `<nav ${cl('menu')} aria-label="${esc(tx(n.label))}"><ul>${li}</ul></nav><details class="menu-m"><summary>Menú</summary><ul>${li}</ul></details>`;
    }
    case 'cart_button':
      return `<a ${cl('bt bt-secondary t-button')} href="#">${esc(tx(n.label))}</a>`;
    case 'embed':
      return `<div ${cl('embed')} role="img" aria-label="${esc(tx(n.title))}">${icono('map-pin', 36)}<span>Mapa (${esc(n.ref)})</span></div>`;
    case 'form': {
      const campos = n.fields.map((f) => {
        const et = esc(tx(f.label)) + (f.required ? ' *' : '');
        const ctl = f.kind === 'textarea' ? '<textarea rows="3"></textarea>' : f.kind === 'select' ? `<select>${(f.options ?? []).map((o) => `<option>${esc(tx(o))}</option>`).join('')}</select>` : `<input type="${f.kind}">`;
        return `<label class="t-label campo">${et}${ctl}</label>`;
      }).join('');
      return `<div ${cl('form')}>${campos}<label class="t-small cons"><input type="checkbox"> ${esc(tx(n.consent))}</label><a class="bt bt-primary t-button" href="#">${esc(tx(n.submit))}</a></div>`;
    }
    case 'table': {
      const cab = n.header.map((h) => `<th scope="col">${esc(tx(h))}</th>`).join('');
      const filas = n.rows.map((f) => `<tr>${f.map((x) => `<td>${esc(tx(x))}</td>`).join('')}</tr>`).join('');
      return `<div ${cl('tabla')} role="region" tabindex="0"><table><caption>${esc(tx(n.caption))}</caption><thead><tr>${cab}</tr></thead><tbody>${filas}</tbody></table></div>`;
    }
    case 'product_grid': {
      const cards = Array.from({ length: n.columns }, () => `<div class="prod"><img src="${placeholderSvg('product')}" alt=""><b>Producto</b><span>12,00 €</span></div>`).join('');
      return `<div ${cl('pgrid')} style="--cols:${n.columns}">${cards}</div>`;
    }
    default:
      return '';
  }
}

const CSS_FIJO = `
.bt{display:inline-flex;align-items:center;gap:8px;padding:12px 20px;border-radius:var(--r-md);text-decoration:none;border:1.5px solid var(--c-primary);line-height:1.2}
.bt-primary{background:var(--c-primary);color:var(--c-on-primary)}
.bt-secondary{background:transparent;color:var(--c-primary)}
.bt-ghost{background:transparent;color:var(--c-primary);border-color:transparent;padding:8px 10px}
.menu ul{display:flex;gap:20px;flex-wrap:wrap}.menu a{text-decoration:none}
.menu-m{display:none}.menu-m summary{cursor:pointer}
@media(max-width:1023.98px){.menu{display:none}.menu-m{display:block}}
.embed{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:8px;background:var(--c-surface);border:1px solid var(--c-border);border-radius:var(--r-md);aspect-ratio:1.78;width:100%;color:var(--c-muted)}
.form{display:flex;flex-direction:column;gap:14px;width:100%}.campo{display:flex;flex-direction:column;gap:6px}
.campo input,.campo select,.campo textarea{font:inherit;padding:10px 12px;border:1px solid var(--c-border);border-radius:var(--r-sm);background:var(--c-background);color:var(--c-text)}
.cons{display:flex;gap:8px;align-items:flex-start}
.tabla{width:100%;overflow-x:auto}table{border-collapse:collapse;width:100%}caption{text-align:left;font-weight:600;padding:0 0 10px;color:var(--c-muted)}
th,td{padding:12px 14px;text-align:left;border-bottom:1px solid var(--c-border)}th{font-weight:600;border-bottom:2px solid var(--c-text)}
.pgrid{display:grid;grid-template-columns:repeat(var(--cols),1fr);gap:20px;width:100%}.prod{display:flex;flex-direction:column;gap:6px}.prod img{width:100%}
blockquote{border-left:3px solid var(--c-primary);padding-left:14px}
@media(max-width:1023.98px){.pgrid{grid-template-columns:repeat(2,1fr)}}
`;

const claves = pedidas.length ? pedidas : readdirSync(dirPl).filter((f) => f.endsWith('.json') && f !== 'temas.json').map((f) => f.replace('.json', ''));
for (const clave of claves) {
  const pl = JSON.parse(readFileSync(join(dirPl, `${clave}.json`), 'utf8'));
  const ir = pl.ir;
  for (const pag of ir.pages) {
    acum.css = {};
    const cuerpo = (pag.show_header && ir.header ? pinta(ir.header, ir, null) : '') + `<main>${pag.sections.map((s) => pinta(s, ir, null)).join('')}</main>` + (pag.show_footer && ir.footer ? pinta(ir.footer, ir, null) : '');
    const css = (acum.css.desktop ?? '') + `@media${PUNTOS.tablet}{${acum.css.tablet ?? ''}}@media${PUNTOS.mobile}{${acum.css.mobile ?? ''}}`;
    const slug = tx(pag.slug);
    const dir = join(salida, clave, slug);
    mkdirSync(dir, { recursive: true });
    const html = `<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${esc(tx(pag.title))}</title><style>${fuentesCss([ir.theme.fonts.heading, ir.theme.fonts.body])}${cssTema(ir)}${CSS_FIJO}${css}</style></head><body>${cuerpo.replaceAll('/__BASE__/', `/${clave}/`)}</body></html>`;
    writeFileSync(join(dir, 'index.html'), html);
  }
}
console.log('vista previa de', claves.length, 'plantillas en', salida);
