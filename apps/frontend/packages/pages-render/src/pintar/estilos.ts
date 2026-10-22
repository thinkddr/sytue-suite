/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CSS del sitio a partir del IR, según la tabla de §4.2 de PAGES-PLAN.md: tokens del tema, estilos de
 * texto, y por cada nodo una clase `n<id>` con su layout (flex), su tamaño (fill / hug / px), su estilo y
 * sus sobrescrituras por punto de corte (tableta ≤ 1023,98 px y móvil ≤ 639,98 px; el móvil hereda de la
 * tableta). Es puro y determinista: mismo IR, mismos bytes. Nada de `style=` en el HTML ni de unidades
 * de pantalla (el lienzo se ajusta a su contenido).
 *
 * Todo valor del IR que llega al CSS se comprueba antes (colores, enumerados, números): el CSS se
 * publica y no puede arrastrar nada que el validador no haya aceptado.
 */

import type { DatosRender } from '../contexto';
import type { Delta, Estilo, Layout, Nodo, Seccion, Sitio, Tamano } from '../ir/tipos';

export type Punto = 'desktop' | 'tablet' | 'mobile';
const PUNTOS: Punto[] = ['desktop', 'tablet', 'mobile'];
const MEDIA: Record<Punto, string> = {
  desktop: '',
  tablet: '@media (max-width:1023.98px)',
  mobile: '@media (max-width:639.98px)',
};

const ALINEAR: Record<string, string> = {
  start: 'flex-start',
  center: 'center',
  end: 'flex-end',
  baseline: 'baseline',
};
const REPARTIR: Record<string, string> = {
  start: 'flex-start',
  center: 'center',
  end: 'flex-end',
  'space-between': 'space-between',
  'space-around': 'space-around',
  'space-evenly': 'space-evenly',
};
const SOMBRAS: Record<string, string> = {
  sm: '0 1px 3px rgba(0,0,0,.14)',
  md: '0 4px 12px rgba(0,0,0,.16)',
  lg: '0 12px 32px rgba(0,0,0,.2)',
};
const TEXTO_ALINEA: Record<string, string> = {
  start: 'start',
  center: 'center',
  end: 'end',
  justify: 'justify',
};

const reColor = /^#[0-9a-f]{6}([0-9a-f]{2})?$/;
const reToken = /^\$[a-z][a-z0-9_]{0,31}$/;

const num = (v: unknown): number | undefined =>
  typeof v === 'number' && Number.isFinite(v) ? v : undefined;
const px = (v: unknown): string | undefined => {
  const n = num(v);
  return n === undefined ? undefined : `${n}px`;
};

/** '#rrggbb(aa)' o '$token'; cualquier otra cosa se descarta. */
export function color(v: unknown): string | undefined {
  if (typeof v !== 'string') return undefined;
  if (reColor.test(v)) return v;
  if (reToken.test(v)) return `var(--c-${v.slice(1)})`;
  return undefined;
}

function radio(v: unknown): string | undefined {
  if (v === '$sm' || v === '$md' || v === '$lg') return `var(--r-${v.slice(1)})`;
  return px(v);
}

/** Propiedades efectivas de un nodo en un punto de corte: base + deltas hasta ese punto. */
export interface Efectivo {
  size: Partial<Tamano>;
  layout: Layout | undefined;
  style: Estilo;
  hidden: boolean;
}

export function efectivo(n: Nodo | Seccion, punto: Punto): Efectivo {
  const e: Efectivo = {
    size: { ...n.size },
    layout: 'layout' in n && n.layout ? { ...n.layout } : undefined,
    style: { ...n.style },
    hidden: n.hidden === true,
  };
  const deltas: Delta[] = [];
  if (punto !== 'desktop' && n.breakpoints?.tablet) deltas.push(n.breakpoints.tablet);
  if (punto === 'mobile' && n.breakpoints?.mobile) deltas.push(n.breakpoints.mobile);
  for (const d of deltas) {
    if (d.hidden !== undefined) e.hidden = d.hidden;
    if (d.size) {
      const tam = e.size as Record<string, unknown>;
      for (const [k, v] of Object.entries(d.size)) {
        if (v === null) delete tam[k];
        else if (v !== undefined) tam[k] = v;
      }
    }
    if (d.layout && e.layout) e.layout = { ...e.layout, ...d.layout };
    if (d.columns !== undefined && e.layout) e.layout = { ...e.layout, columns: d.columns };
    if (d.style) {
      const est = e.style as Record<string, unknown>;
      for (const [k, v] of Object.entries(d.style)) {
        if (v === null) delete est[k];
        else if (v !== undefined) est[k] = v;
      }
    }
  }
  return e;
}

const DEFECTO: Record<string, { width: 'fill' | 'hug' | number; height: 'fill' | 'hug' | number }> =
  {
    button: { width: 'hug', height: 'hug' },
    menu: { width: 'hug', height: 'hug' },
    cart_button: { width: 'hug', height: 'hug' },
    language_switcher: { width: 'hug', height: 'hug' },
    icon: { width: 24, height: 24 },
  };

function reglasLayout(l: Layout | undefined): string[] {
  if (!l) return [];
  const r = ['display:flex', `flex-direction:${l.direction === 'row' ? 'row' : 'column'}`];
  const gap = px(l.gap);
  if (gap) r.push(`gap:${gap}`);
  const rg = px(l.row_gap);
  if (l.wrap && rg) r.push(`row-gap:${rg}`);
  if (Array.isArray(l.padding)) r.push(`padding:${l.padding.map((x) => px(x) ?? '0').join(' ')}`);
  r.push(`align-items:${ALINEAR[l.align] ?? 'flex-start'}`);
  r.push(`justify-content:${REPARTIR[l.justify] ?? 'flex-start'}`);
  if (l.wrap) r.push('flex-wrap:wrap');
  return r;
}

function reglasEstilo(s: Estilo): string[] {
  const r: string[] = [];
  const bg = s.background;
  if (typeof bg === 'string') {
    const c = color(bg);
    if (c) r.push(`background:${c}`);
  }
  const c = color(s.color);
  if (c) r.push(`color:${c}`);
  if (s.border) {
    const bc = color(s.border.color);
    const w = px(s.border.width);
    if (bc && w) r.push(`border:${w} ${s.border.style === 'dashed' ? 'dashed' : 'solid'} ${bc}`);
  }
  const rad = radio(s.radius);
  if (rad) r.push(`border-radius:${rad}`);
  if (s.overflow === 'hidden') r.push('overflow:hidden');
  if (s.shadow && SOMBRAS[s.shadow]) r.push(`box-shadow:${SOMBRAS[s.shadow]}`);
  const op = num(s.opacity);
  if (op !== undefined) r.push(`opacity:${op}`);
  const t = s.text;
  if (t) {
    if (t.align && TEXTO_ALINEA[t.align]) r.push(`text-align:${TEXTO_ALINEA[t.align]}`);
    if (t.transform === 'uppercase' || t.transform === 'none')
      r.push(`text-transform:${t.transform}`);
    if (t.weight && [400, 500, 600, 700, 800].includes(t.weight)) r.push(`font-weight:${t.weight}`);
    const sz = px(t.size);
    if (sz) r.push(`font-size:${sz}`);
    const lh = num(t.line_height);
    if (lh !== undefined) r.push(`line-height:${lh}`);
    const ls = num(t.letter_spacing);
    if (ls !== undefined) r.push(`letter-spacing:${ls}em`);
    if (t.italic === true) r.push('font-style:italic');
    if (t.font === 'heading' || t.font === 'body') r.push(`font-family:var(--f-${t.font})`);
  }
  return r;
}

function reglasTamano(
  n: Nodo | Seccion,
  e: Efectivo,
  padre: { layout: Layout | undefined } | null,
): string[] {
  const r: string[] = [];
  const tam = { ...(DEFECTO[n.type] ?? { width: 'fill', height: 'hug' }), ...e.size } as Record<
    string,
    unknown
  >;
  if (padre) {
    const pl = padre.layout;
    const fila = pl?.direction === 'row';
    const principal = fila ? 'width' : 'height';
    const cruzado = fila ? 'height' : 'width';
    const cols = pl?.wrap && pl.columns ? pl.columns : 0;
    if (cols) {
      const gap = num(pl?.gap) ?? 0;
      r.push(`flex:0 0 calc((100% - ${(cols - 1) * gap}px)/${cols})`, 'min-width:0');
    } else {
      const m = tam[principal];
      if (m === 'fill') {
        r.push('flex:1 1 0%');
        if (fila && e.size.min_width == null) r.push('min-width:0');
        if (!fila && e.size.min_height == null) r.push('min-height:0');
      } else if (typeof m === 'number') {
        r.push(`flex:0 0 ${m}px`, `${principal}:${m}px`, 'max-width:100%');
      } else r.push('flex:0 0 auto');
      const c = tam[cruzado];
      if (c === 'fill') r.push('align-self:stretch');
      else if (typeof c === 'number') r.push(`${cruzado}:${c}px`);
    }
  } else if (n.type !== 'section') {
    // Sin padre conocido (raíz de componente): el ancho lo manda el contenedor.
    if (typeof tam.width === 'number') r.push(`width:${tam.width}px`, 'max-width:100%');
  }
  for (const [prop, v] of [
    ['min-width', e.size.min_width],
    ['max-width', e.size.max_width],
    ['min-height', e.size.min_height],
    ['max-height', e.size.max_height],
  ] as const) {
    const p = px(v);
    if (p) r.push(`${prop}:${p}`);
  }
  const ar = num(e.size.aspect_ratio);
  if (ar !== undefined && ar > 0) r.push(`aspect-ratio:${ar}`);
  return r;
}

/** Reglas de un nodo no sección en un punto. */
function reglasNodo(n: Nodo, e: Efectivo, padre: { layout: Layout | undefined } | null): string {
  const r = [...reglasTamano(n, e, padre)];
  if (n.type === 'frame' || n.type === 'form') r.push(...reglasLayout(e.layout));
  if (
    n.type === 'image' &&
    (padre === null || e.size.width === undefined || e.size.width === 'fill')
  )
    r.push('width:100%', 'height:auto');
  if (n.type === 'image') r.push(`object-fit:${n.fit === 'contain' ? 'contain' : 'cover'}`);
  r.push(...reglasEstilo(e.style));
  if (e.hidden) r.push('display:none');
  return r.join(';');
}

function reglasSeccion(s: Seccion, e: Efectivo): { fuera: string; dentro: string } {
  const l = e.layout;
  const fuera: string[] = [];
  const bg = color(typeof e.style.background === 'string' ? e.style.background : undefined);
  if (bg) fuera.push(`background:${bg}`);
  const c = color(e.style.color);
  if (c) fuera.push(`color:${c}`);
  if (e.style.border) {
    // ponytail: la raya de una sección es solo la de abajo (cabecera con filete); un borde completo
    // a todo el ancho de la página no tiene uso. Si hace falta, un campo propio en el IR.
    const bc = color(e.style.border.color);
    const w = px(e.style.border.width);
    if (bc && w) fuera.push(`border-bottom:${w} solid ${bc}`);
  }
  const alto = e.size.height;
  if (typeof alto === 'number') fuera.push(`min-height:${alto}px`);
  const flex = reglasLayout(l);
  if (s.full_bleed) {
    fuera.push(...flex);
    if (e.hidden) fuera.push('display:none');
    return { fuera: fuera.join(';'), dentro: '' };
  }
  const pad = flex.find((x) => x.startsWith('padding:'));
  if (pad) fuera.push(pad);
  if (e.hidden) fuera.push('display:none');
  const dentro = [
    ...flex.filter((x) => !x.startsWith('padding:')),
    'max-width:var(--sp-container)',
    'margin-inline:auto',
    'width:100%',
  ];
  return { fuera: fuera.join(';'), dentro: dentro.join(';') };
}

/** Recorre un árbol y apunta, por punto, las reglas que cambian respecto al punto anterior. */
function acumular(
  n: Nodo | Seccion,
  padre: Nodo | Seccion | null,
  salida: Record<Punto, string[]>,
): void {
  const cls = `.n${n.id}`;
  let previo: string[] | null = null;
  for (const punto of PUNTOS) {
    const e = efectivo(n, punto);
    let actual: string[];
    if (n.type === 'section') {
      const { fuera, dentro } = reglasSeccion(n, e);
      actual = [fuera, dentro];
    } else {
      actual = [reglasNodo(n, e, padre ? { layout: efectivo(padre, punto).layout } : null)];
    }
    const cambia = previo === null || actual.some((x, i) => x !== previo?.[i]);
    if (cambia) {
      const sels = n.type === 'section' ? [cls, `${cls}>.sp-c`] : [cls];
      const reglas = actual
        .map((cuerpo, i) => (cuerpo === '' ? '' : `${sels[i]}{${cuerpo}}`))
        .join('');
      if (reglas !== '') salida[punto].push(reglas);
    }
    previo = actual;
  }
  if ('children' in n) for (const h of n.children) acumular(h, n, salida);
}

/** CSS de todos los nodos del sitio (cabecera, pie, páginas y maestros de componentes). */
export function cssNodos(ir: Sitio): string {
  const salida: Record<Punto, string[]> = { desktop: [], tablet: [], mobile: [] };
  if (ir.header) acumular(ir.header, null, salida);
  if (ir.footer) acumular(ir.footer, null, salida);
  for (const p of ir.pages) for (const s of p.sections) acumular(s, null, salida);
  for (const c of ir.components) acumular(c.root, null, salida);
  let css = salida.desktop.join('');
  for (const punto of ['tablet', 'mobile'] as const) {
    if (salida[punto].length > 0) css += `${MEDIA[punto]}{${salida[punto].join('')}}`;
  }
  return css;
}

const FUENTE_SEGURA = /[^A-Za-z0-9 -]/g;

/** Tokens del tema, familias (con sus `@font-face` si hay datos) y estilos de texto. */
export function cssTema(
  ir: Sitio,
  datos?: DatosRender,
  recurso = (n: string) => `/_astro/${n}`,
): string {
  const t = ir.theme;
  const claves = Object.keys(t.colors).sort();
  const tokens = claves
    .map((k) => {
      const v = t.colors[k];
      return v !== undefined && reColor.test(v) && /^[a-z][a-z0-9_]{0,31}$/.test(k)
        ? `--c-${k}:${v}`
        : '';
    })
    .filter(Boolean)
    .join(';');
  const familia = (id: string): { pila: string; faces: string } => {
    const f = datos?.fonts[id];
    if (!f) return { pila: 'system-ui,sans-serif', faces: '' };
    const nombre = f.name.replace(FUENTE_SEGURA, '');
    const faces = f.faces
      .map(
        (x) =>
          `@font-face{font-family:"${nombre}";src:url(${recurso(x.file)}) format("woff2");font-style:${x.style === 'italic' ? 'italic' : 'normal'};font-weight:${x.weight.replace(/[^0-9 ]/g, '')};unicode-range:${x.unicode_range.replace(/[^0-9A-Fa-f,+\- ?U]/g, '')};font-display:swap}`,
      )
      .join('');
    return { pila: `"${nombre}",${f.fallback === 'serif' ? 'serif' : 'sans-serif'}`, faces };
  };
  const hd = familia(t.fonts.heading);
  const bd = familia(t.fonts.body);
  let css = hd.faces + (t.fonts.body === t.fonts.heading ? '' : bd.faces);
  css += `:root{${tokens};--r-sm:${num(t.radius?.sm) ?? 4}px;--r-md:${num(t.radius?.md) ?? 8}px;--r-lg:${num(t.radius?.lg) ?? 12}px;--f-heading:${hd.pila};--f-body:${bd.pila}}`;
  for (const [clave, e] of Object.entries(t.text_styles ?? {})) {
    if (!/^[a-z0-9]+$/.test(clave) || !e) continue;
    const fam = e.font === 'heading' ? 'var(--f-heading)' : 'var(--f-body)';
    const base = `font-family:${fam};line-height:${num(e.line_height) ?? 1.4};font-weight:${e.weight};letter-spacing:${num(e.letter_spacing) ?? 0}em${e.transform === 'uppercase' ? ';text-transform:uppercase' : ''}`;
    const [d, tb, mv] = e.size;
    css += `.t-${clave}{${base};font-size:${d}px}`;
    css += `${MEDIA.tablet}{.t-${clave}{font-size:${tb}px}}${MEDIA.mobile}{.t-${clave}{font-size:${mv}px}}`;
  }
  return css;
}
