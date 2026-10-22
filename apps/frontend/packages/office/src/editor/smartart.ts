/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SmartArt del EDITOR DE DOCUMENTOS (kind document). El HTML del folio ES el
 * modelo (§6.5), así que el diagrama vive como un bloque autocontenido
 * `<figure data-smartart data-layout data-items contenteditable=false>` cuyo
 * SVG se HORNEA dentro del HTML (persiste y viaja al export, igual que notas al
 * pie/leyendas/columnas). La fuente de verdad son `data-layout` + `data-items`;
 * el SVG es su render y se regenera al editar.
 *
 * La geometría (posiciones/tamaños de nodos y conectores) NO se reimplementa:
 * se reutiliza tal cual smartArtLayout de slides/smartart.ts (misma lógica pura
 * clean-room ya validada). Aquí solo va: (de)serializado de los elementos y el
 * volcado de la geometría a una CADENA SVG sobria (nodos con relleno/borde en
 * currentColor —la tinta del documento, segura en export—, conectores, texto
 * ajustado en foreignObject). Módulo puro (sin Preact ni DOM): el self-test lo
 * cubre con bun.
 */
import { smartArtLayout } from './slides/smartart';
import type { SmartArtItem, SmartArtLayout } from './types';
import { editorT, type DocsKey } from '../i18n';

/* ---------- (de)serializado de elementos (data-items) ---------- */

const LAYOUTS: readonly SmartArtLayout[] = ['list', 'process', 'cycle', 'hierarchy'];

/** Normaliza una disposición cualquiera a una válida ('list' por defecto). */
export const sanitizeSmartArtLayout = (raw: string | null | undefined): SmartArtLayout =>
  LAYOUTS.includes(raw as SmartArtLayout) ? (raw as SmartArtLayout) : 'list';

/**
 * Serializa los elementos para el atributo `data-items`: solo `text` (string) y,
 * si aporta sangría de jerarquía, `level` (entero ≥1). Round-trip con
 * parseSmartArtItems. NO escapa: el escape para el contexto de atributo lo hace
 * setAttribute (edición) o escapeAttr (inserción por cadena).
 */
export const serializeSmartArtItems = (items: SmartArtItem[]): string =>
  JSON.stringify(
    items.map((it) => {
      const level = Math.max(0, Math.floor(Number(it.level) || 0));
      return level > 0 ? { text: String(it.text ?? ''), level } : { text: String(it.text ?? '') };
    }),
  );

/**
 * Parsea `data-items` de forma TOLERANTE: JSON inválido o no-array → []; cada
 * entrada aporta `text` (string, '' si falta) y `level` entero ≥0 (ausente si 0).
 * Frontera de datos: un modelo viejo/corrupto no revienta el render.
 */
export const parseSmartArtItems = (raw: string | null | undefined): SmartArtItem[] => {
  if (!raw) return [];
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(data)) return [];
  return data.map((x) => {
    const o = (x ?? {}) as { text?: unknown; level?: unknown };
    const text = typeof o.text === 'string' ? o.text : '';
    const level = Math.max(0, Math.floor(Number(o.level) || 0));
    return level > 0 ? { text, level } : { text };
  });
};

/** Elementos de arranque de un diagrama nuevo, por disposición (la jerarquía
 *  nace con raíz + dos hijos para que se vea el árbol de inmediato). */
export const DEFAULT_SMARTART_ITEMS: Record<SmartArtLayout, SmartArtItem[]> = {
  list: [{ text: 'Primer elemento' }, { text: 'Segundo elemento' }, { text: 'Tercer elemento' }],
  process: [{ text: 'Inicio' }, { text: 'Desarrollo' }, { text: 'Resultado' }],
  cycle: [{ text: 'Planificar' }, { text: 'Hacer' }, { text: 'Revisar' }, { text: 'Ajustar' }],
  hierarchy: [
    { text: 'Dirección' },
    { text: 'Equipo A', level: 1 },
    { text: 'Equipo B', level: 1 },
  ],
};

/* ---------- escape para inyección segura en HTML (anti-XSS) ---------- */

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));
/** Redondeo a centésima de px: paths compactos (mismo criterio que slides). */
const cpx = (n: number): number => Math.round(n * 100) / 100;

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};
/** Escapa texto para contenido HTML/SVG (texto de datos del usuario). */
const esc = (s: string): string => s.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
/** Escapa un valor para un atributo entre comillas dobles. */
const escAttr = (s: string): string => s.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);

/* ---------- volcado de la geometría a SVG (cadena) ---------- */

/** Ancho fijo del lienzo del diagrama (espacio de coordenadas del viewBox); la
 *  figura se pinta a width:100%, así que este ancho solo fija el aspecto. */
const VIEW_W = 640;

/** Rótulo accesible por disposición (aria-label del SVG). */
const ARIA: Record<SmartArtLayout, DocsKey> = {
  list: 'smartart.kind.list',
  process: 'smartart.kind.process',
  cycle: 'smartart.kind.cycle',
  hierarchy: 'smartart.kind.hierarchy',
};

/** Alto del viewBox por disposición: crece con los elementos donde apilan
 *  (lista) o por filas (jerarquía) para que los nodos no se aplasten; fijo donde
 *  reparten a lo ancho/en anillo (proceso/ciclo). */
const viewHeight = (layout: SmartArtLayout, items: SmartArtItem[]): number => {
  const n = Math.max(1, items.length);
  switch (layout) {
    case 'process':
      return 170;
    case 'cycle':
      return 380;
    case 'hierarchy': {
      const maxLevel = items.reduce((m, it) => Math.max(m, Math.floor(Number(it.level) || 0)), 0);
      return clamp((Math.min(maxLevel, 6) + 1) * 130, 170, 900);
    }
    case 'list':
    default:
      return clamp(n * 52 + 24, 120, 900);
  }
};

/** SVG (cadena) de un conector: codo ortogonal sin punta (jerarquía) o flecha
 *  recta con punta (proceso/ciclo). currentColor con opacidades sobrias. */
const connectorSvg = (
  c: ReturnType<typeof smartArtLayout>['connectors'][number],
  headSize: number,
): string => {
  if (c.kind === 'elbow') {
    const my = cpx((c.y1 + c.y2) / 2);
    return (
      `<path d="M${cpx(c.x1)} ${cpx(c.y1)}V${my}H${cpx(c.x2)}V${cpx(c.y2)}" fill="none" ` +
      `stroke="currentColor" stroke-opacity="0.42" stroke-width="1.5" stroke-linejoin="round"/>`
    );
  }
  const ang = Math.atan2(c.y2 - c.y1, c.x2 - c.x1);
  const h1x = cpx(c.x2 - headSize * Math.cos(ang - Math.PI / 7));
  const h1y = cpx(c.y2 - headSize * Math.sin(ang - Math.PI / 7));
  const h2x = cpx(c.x2 - headSize * Math.cos(ang + Math.PI / 7));
  const h2y = cpx(c.y2 - headSize * Math.sin(ang + Math.PI / 7));
  return (
    `<g stroke="currentColor" stroke-opacity="0.5" stroke-width="2" fill="none">` +
    `<line x1="${cpx(c.x1)}" y1="${cpx(c.y1)}" x2="${cpx(c.x2)}" y2="${cpx(c.y2)}" stroke-linecap="round"/>` +
    `<path d="M${h1x} ${h1y}L${cpx(c.x2)} ${cpx(c.y2)}L${h2x} ${h2y}" stroke-linecap="round" stroke-linejoin="round"/>` +
    `</g>`
  );
};

/** SVG (cadena) de un nodo: forma sobria (relleno tenue + borde) + texto
 *  centrado y ajustado (foreignObject). `bullet` antepone viñeta en la lista. */
const nodeSvg = (
  nd: ReturnType<typeof smartArtLayout>['nodes'][number],
  text: string,
  bullet: boolean,
): string => {
  const fs = clamp(Math.min(nd.h * 0.32, nd.w * 0.15), 7, 20);
  const shape =
    nd.shape === 'ellipse'
      ? `<ellipse cx="${cpx(nd.x + nd.w / 2)}" cy="${cpx(nd.y + nd.h / 2)}" rx="${cpx(nd.w / 2)}" ry="${cpx(nd.h / 2)}" fill="currentColor" fill-opacity="0.08" stroke="currentColor" stroke-opacity="0.4" stroke-width="1.5"/>`
      : `<rect x="${cpx(nd.x)}" y="${cpx(nd.y)}" width="${cpx(nd.w)}" height="${cpx(nd.h)}" rx="${cpx(clamp(Math.min(nd.w, nd.h) * 0.16, 2, 10))}" fill="currentColor" fill-opacity="0.08" stroke="currentColor" stroke-opacity="0.4" stroke-width="1.5"/>`;
  const padV = cpx(clamp(nd.h * 0.08, 1, 6));
  const padH = cpx(clamp(nd.w * 0.06, 3, 14));
  const style =
    `box-sizing:border-box;width:100%;height:100%;display:flex;align-items:center;` +
    `justify-content:${bullet ? 'flex-start' : 'center'};text-align:${bullet ? 'left' : 'center'};` +
    `gap:0.4em;padding:${padV}px ${padH}px;font-family:var(--font-sans,Inter,sans-serif);` +
    `font-size:${cpx(fs)}px;font-weight:500;line-height:1.2;color:currentColor;overflow:hidden;word-break:break-word`;
  const dot = bullet
    ? '<span aria-hidden="true" style="opacity:0.55;flex:0 0 auto">&#8226;</span>'
    : '';
  return (
    `<g>${shape}<foreignObject x="${cpx(nd.x)}" y="${cpx(nd.y)}" width="${cpx(nd.w)}" height="${cpx(nd.h)}">` +
    `<div xmlns="http://www.w3.org/1999/xhtml" style="${style}">${dot}<span style="min-width:0">${esc(text)}</span></div>` +
    `</foreignObject></g>`
  );
};

/**
 * SVG (cadena) del diagrama para la disposición y los elementos dados. Reutiliza
 * smartArtLayout para la geometría (no la duplica). Sin elementos (o caja
 * degenerada) devuelve un SVG con un aviso sobrio. Puro y exportado para el
 * self-test (bun test).
 */
export const smartArtSvg = (layout: SmartArtLayout, items: SmartArtItem[]): string => {
  const w = VIEW_W;
  const h = viewHeight(layout, items);
  const geo = smartArtLayout(layout, items, w, h);
  const header =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${cpx(h)}" width="100%" ` +
    `style="display:block;height:auto" role="img" aria-label="${esc(editorT()('smartart.aria', { kind: editorT()(ARIA[layout]) }))}">`;
  if (geo.nodes.length === 0) {
    return (
      `${header}<text x="${w / 2}" y="${cpx(h / 2)}" text-anchor="middle" dominant-baseline="middle" ` +
      `fill="currentColor" fill-opacity="0.6" font-family="var(--font-sans,Inter,sans-serif)" font-size="16">` +
      `${esc(editorT()('smartart.empty'))}</text></svg>`
    );
  }
  const headSize = clamp(Math.min(w, h) * 0.03, 4, 12);
  const bullet = layout === 'list';
  const parts = [
    ...geo.connectors.map((c) => connectorSvg(c, headSize)),
    ...geo.nodes.map((nd) => nodeSvg(nd, items[nd.i]?.text ?? '', bullet)),
  ];
  return `${header}${parts.join('')}</svg>`;
};

/** Estilo en línea de la figura (el HTML ES el modelo: viaja al export). Sobrio,
 *  sin borde propio (los nodos ya lo llevan); centrado con aire vertical. */
const FIGURE_STYLE = 'margin:1em auto;width:100%';

/**
 * HTML completo del bloque SmartArt para INSERTAR en el folio: figura
 * contenteditable=false con `data-layout`/`data-items` (fuente de verdad) y el
 * SVG horneado. Escapa el texto del usuario (anti-XSS). `tmpId` marca la figura
 * recién insertada para poder re-localizarla (se retira tras engancharla). Puro
 * y exportado para el self-test.
 */
export const smartArtFigureHtml = (
  layout: SmartArtLayout,
  items: SmartArtItem[],
  tmpId?: string,
): string => {
  const tmp = tmpId ? ` data-sa-tmp="${escAttr(tmpId)}"` : '';
  return (
    `<figure data-smartart data-layout="${escAttr(layout)}" ` +
    `data-items="${escAttr(serializeSmartArtItems(items))}"${tmp} contenteditable="false" ` +
    `style="${FIGURE_STYLE}">${smartArtSvg(layout, items)}</figure>`
  );
};
