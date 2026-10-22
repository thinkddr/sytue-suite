/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bordes de página + marca de agua — AYUDAS PURAS (sin DOM). El pegamento con el
 * folio (crear/quitar la capa, leer los ajustes de vuelta y emitir `input`) vive
 * en ribbon/tabs/Disposicion.tsx; aquí solo la lógica no trivial y testeable:
 * sanear los ajustes y HORNEAR la estructura + CSS de cada capa (con escape
 * anti-XSS del texto/URL del usuario, que es la frontera de confianza). Ambas
 * capas viven EMBEBIDAS en el HTML del folio —que ES el modelo (§6.5)—, así que
 * persisten con el autosave y viajan autocontenidas al export (mismo criterio
 * que encabezado/pie, columnas y SmartArt): un `<div>` posicionado, con los
 * ajustes en atributos `data-*` (fuente de verdad) y el render horneado en el
 * atributo `style` (una imagen SVG de fondo, embebida como data-URI).
 *
 * La marca de agua se pinta DETRÁS del contenido (z-index:-1; el folio tiene
 * `isolation:isolate` en editor.css, así que pinta sobre el papel pero bajo el
 * texto, igual que una imagen anclada «detrás del texto») y se repite una vez
 * por alto de página (mosaico vertical), centrada. El borde rodea el área de
 * contenido (recuadro dentro del margen), sin capturar ratón ni selección.
 *
 * Ejecutar el self-test: cd apps/frontend && bun test packages/office/test/pagedecor.test.ts
 */
import type { DocsKey } from '../i18n';
import { PAGE_W, PAGE_H } from './canvas/metrics';
import type { PageSetup } from './types';

/* ---------- La página del documento: tamaño, márgenes y color del papel ----------
 * Viven en una capa vacía al principio del folio, `div[data-sytue-page]`, con todo en px
 * en sus `data-*`. La escriben la cinta (Disposición), rico y el importador de .docx, y la
 * lee la exportación. Sin capa, la página es la de siempre: A4 vertical con 96 px. */

export type PageSizeId = 'a4' | 'carta' | 'legal';

/** «A4» y «Legal» son nombres de formato: no se traducen (§13.1). «Carta»/«Letter»
 *  sí, porque cada lengua tiene el suyo. */
export const PAGE_SIZES: Record<PageSizeId, { labelKey: DocsKey; w: number; h: number }> = {
  a4: { labelKey: 'layout.size.a4', w: PAGE_W, h: PAGE_H }, // 210 × 297 mm (el nativo del lienzo)
  carta: { labelKey: 'layout.size.letter', w: 816, h: 1056 }, // 8,5 × 11 in
  legal: { labelKey: 'layout.size.legal', w: 816, h: 1344 }, // 8,5 × 14 in
};

/** Con todas las claves: setPageSetup MEZCLA, y una que falte dejaría la de antes. */
export const PAGINA_POR_DEFECTO: PageSetup = {
  margin: 96,
  landscape: false,
  size: undefined,
  color: undefined,
  width: undefined,
  height: undefined,
  margins: undefined,
};

/** Ancho y alto del folio, ya orientados. */
export function medidasPagina(p: PageSetup): { w: number; h: number } {
  if (p.width && p.height) return { w: p.width, h: p.height };
  const t = PAGE_SIZES[p.size ?? 'a4'];
  return p.landscape ? { w: t.h, h: t.w } : { w: t.w, h: t.h };
}

/** Márgenes: arriba, derecha, abajo, izquierda. */
export function margenesPagina(p: PageSetup): [number, number, number, number] {
  return p.margins ?? [p.margin, p.margin, p.margin, p.margin];
}

const redondo = (n: number): string => String(Math.round(n * 100) / 100);

/** La capa que guarda la página, o '' si es la de por defecto (no hace falta guardarla). */
export function paginaHtml(p: PageSetup): string {
  const { w, h } = medidasPagina(p);
  const m = margenesPagina(p);
  const d = PAGINA_POR_DEFECTO;
  if (w === PAGE_W && h === PAGE_H && m.every((x) => x === d.margin) && !p.color) return '';
  const color =
    p.color && /^#[0-9a-f]{6}$/i.test(p.color) ? ` data-color="${p.color.toUpperCase()}"` : '';
  return (
    `<div data-sytue-page="" data-width="${redondo(w)}" data-height="${redondo(h)}" ` +
    `data-margins="${m.map(redondo).join(' ')}"${color} contenteditable="false" aria-hidden="true"></div>`
  );
}

/** La página que guarda la capa (o la de por defecto, sin capa). Lo que no se entiende vale
 *  lo del folio. Un tamaño conocido vuelve como `size` (a 2 px), y unos márgenes iguales,
 *  como `margin`: así la cinta los enseña como los suyos. Es `paginaDe` de rico. */
export function leerPagina(el: Element | null): PageSetup {
  if (!el) return { ...PAGINA_POR_DEFECTO };
  const num = (v: string | null | undefined, def: number, lo: number, hi: number): number => {
    const n = Number.parseFloat(v ?? '');
    return Number.isFinite(n) && n >= lo && n <= hi ? n : def;
  };
  const w = num(el.getAttribute('data-width'), PAGE_W, 96, 4000);
  const h = num(el.getAttribute('data-height'), PAGE_H, 96, 4000);
  const crudos = (el.getAttribute('data-margins') ?? '').trim().split(/\s+/);
  const m = [0, 1, 2, 3].map((i) => num(crudos[i], 96, 0, 2000)) as [
    number,
    number,
    number,
    number,
  ];
  const p: PageSetup = { ...PAGINA_POR_DEFECTO, landscape: w > h, width: w, height: h, margins: m };
  if (m.every((x) => x === m[0])) {
    p.margin = m[0];
    p.margins = undefined;
  }
  const cerca = (a: number, b: number): boolean => Math.abs(a - b) < 2;
  for (const [id, t] of Object.entries(PAGE_SIZES) as [PageSizeId, { w: number; h: number }][]) {
    if ((cerca(t.w, w) && cerca(t.h, h)) || (cerca(t.h, w) && cerca(t.w, h))) {
      p.size = id;
      p.width = undefined;
      p.height = undefined;
    }
  }
  const color = el.getAttribute('data-color');
  if (color && /^#[0-9a-f]{6}$/i.test(color)) p.color = color.toUpperCase();
  return p;
}

/* ---------- escape (frontera de confianza: texto/URL de usuario) ---------- */

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

/** Escapa texto para contenido XML/HTML (texto de la marca de agua). */
const escXml = (s: string): string => s.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
/** Escapa un valor para un atributo entre comillas dobles. */
const escAttr = (s: string): string => s.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);

/**
 * Codifica un SVG para incrustarlo como data-URI en `background-image:url('…')`.
 * Se codifica primero `%` (para no romper los `%xx` propios) y luego los
 * caracteres que romperían la url() o el atributo `style="…"` contenedor: `<`,
 * `>`, `#`, `"` y `&`. El SVG usa comillas dobles internas (→ %22), de modo que
 * la url() puede ir entre comillas simples y el `style="…"` externo (comillas
 * dobles) queda intacto. `+` se codifica (→ %2B) para que el base64 anidado de
 * una marca de IMAGEN no se malinterprete como espacio en algún motor. Pura y
 * determinista (la cubre el self-test).
 */
const encodeSvgForCss = (svg: string): string =>
  svg
    .replace(/%/g, '%25')
    .replace(/</g, '%3C')
    .replace(/>/g, '%3E')
    .replace(/#/g, '%23')
    .replace(/"/g, '%22')
    .replace(/&/g, '%26')
    .replace(/\+/g, '%2B')
    .replace(/\n/g, '%20');

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));
/** Redondeo a centésima (CSS/paths compactos). */
const round2 = (n: number): number => Math.round(n * 100) / 100;

const HEX_RE = /^#[0-9a-fA-F]{6}$/;
/** Color #RRGGBB válido o el de reserva (los colores vienen de paletas del ribbon). */
const safeHex = (raw: unknown, fallback: string): string => {
  const s = typeof raw === 'string' ? raw.trim() : '';
  return HEX_RE.test(s) ? s.toUpperCase() : fallback;
};

/** Solo mapas de bits embebidos: se RECHAZA `image/svg+xml` (podría portar
 *  script) y cualquier esquema que no sea una data-URI de imagen ráster. */
const SAFE_IMAGE_RE = /^data:image\/(png|jpe?g|gif|webp|avif);base64,[a-z0-9+/=\s]+$/i;
export const isSafeWatermarkImage = (raw: unknown): raw is string =>
  typeof raw === 'string' && SAFE_IMAGE_RE.test(raw.trim());

/** Número finito coaccionado desde string/number, o el de reserva. */
const num = (raw: unknown, fallback: number): number => {
  const n = typeof raw === 'number' ? raw : typeof raw === 'string' ? parseFloat(raw) : NaN;
  return Number.isFinite(n) ? n : fallback;
};

/** Booleano coaccionado desde '1'/'true'/true (atributos data-* o estado UI). */
const bool = (raw: unknown): boolean => raw === true || raw === '1' || raw === 'true';

/* ==================================================================== */
/* Marca de agua                                                         */
/* ==================================================================== */

export type WatermarkKind = 'text' | 'image';

/** Ajustes de la marca de agua (fuente de verdad: atributos `data-*`). */
export interface WatermarkSpec {
  kind: WatermarkKind;
  /** Texto (kind 'text'); '' en 'image'. */
  text: string;
  /** data-URI de imagen ráster (kind 'image'); ausente en 'text'. */
  image?: string;
  /** Color del texto #RRGGBB (kind 'text'). */
  color: string;
  /** Opacidad de la capa 0..1 (tenue). */
  opacity: number;
  /** kind 'text': tamaño de fuente en px del lienzo 794×1123. kind 'image': % del ancho de página. */
  size: number;
  /** Diagonal (−45°) o horizontal. */
  diagonal: boolean;
}

/** Presets de texto rápidos (paridad Word: BORRADOR/CONFIDENCIAL…). El texto acaba
 *  DENTRO del folio, así que son claves de catálogo: una marca de agua en español
 *  sobre un documento alemán sería un defecto, no una decisión. */
export const WATERMARK_PRESETS = [
  'watermark.preset.draft',
  'watermark.preset.confidential',
  'watermark.preset.doNotCopy',
  'watermark.preset.urgent',
  'watermark.preset.sample',
] as const satisfies readonly DocsKey[];

export const DEFAULT_WATERMARK_COLOR = '#94A3B8';
const DEFAULT_TEXT_SIZE = 120;
const DEFAULT_IMAGE_SIZE = 60;

/** Lienzo del mosaico = folio A4 vertical @96dpi; una marca por alto de página.
 *  ponytail: alto fijo A4; en apaisado/otros tamaños el mosaico no cuadra por
 *  página (como encabezado/pie y numeración, que asumen folio continuo). */
export const WATERMARK_TILE_W = PAGE_W;
export const WATERMARK_TILE_H = PAGE_H;

/**
 * Sanea unos ajustes cualesquiera (del panel o releídos de los `data-*`) a un
 * WatermarkSpec, o `null` si la marca está vacía (sin texto ni imagen válida).
 * Frontera de datos: coacciona strings/números y descarta lo inválido.
 */
export const sanitizeWatermarkSpec = (raw: {
  kind?: unknown;
  text?: unknown;
  image?: unknown;
  color?: unknown;
  opacity?: unknown;
  size?: unknown;
  diagonal?: unknown;
}): WatermarkSpec | null => {
  const isImage = raw.kind === 'image' && isSafeWatermarkImage(raw.image);
  if (isImage) {
    return {
      kind: 'image',
      text: '',
      image: String(raw.image).trim(),
      color: safeHex(raw.color, DEFAULT_WATERMARK_COLOR),
      opacity: round2(clamp(num(raw.opacity, 0.15), 0.02, 1)),
      size: Math.round(clamp(num(raw.size, DEFAULT_IMAGE_SIZE), 10, 100)),
      diagonal: bool(raw.diagonal),
    };
  }
  const text = typeof raw.text === 'string' ? raw.text.trim() : '';
  if (text === '') return null; // ni texto ni imagen → sin marca
  return {
    kind: 'text',
    text,
    color: safeHex(raw.color, DEFAULT_WATERMARK_COLOR),
    opacity: round2(clamp(num(raw.opacity, 0.15), 0.02, 1)),
    size: Math.round(clamp(num(raw.size, DEFAULT_TEXT_SIZE), 24, 400)),
    diagonal: bool(raw.diagonal),
  };
};

/** SVG (cadena) de UNA baldosa de marca de agua, centrada en el lienzo A4 y
 *  rotada −45° si es diagonal. Escapa el texto (anti-XSS); la imagen ya está
 *  saneada a data-URI ráster por sanitizeWatermarkSpec. */
export const watermarkSvg = (spec: WatermarkSpec): string => {
  const cx = WATERMARK_TILE_W / 2;
  const cy = WATERMARK_TILE_H / 2;
  const rot = spec.diagonal ? ` transform="rotate(-45 ${cx} ${cy})"` : '';
  const head =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${WATERMARK_TILE_W} ${WATERMARK_TILE_H}" ` +
    `preserveAspectRatio="xMidYMid meet">`;
  if (spec.kind === 'image' && spec.image) {
    const scale = clamp(spec.size, 10, 100) / 100;
    const bw = round2(WATERMARK_TILE_W * scale);
    const bh = round2(WATERMARK_TILE_H * scale);
    const bx = round2((WATERMARK_TILE_W - bw) / 2);
    const by = round2((WATERMARK_TILE_H - bh) / 2);
    // href de imagen: el data-URI ráster va escapado como atributo XML.
    return (
      `${head}<image href="${escXml(spec.image)}" x="${bx}" y="${by}" width="${bw}" height="${bh}" ` +
      `preserveAspectRatio="xMidYMid meet"${rot}/></svg>`
    );
  }
  const fs = clamp(spec.size, 24, 400);
  return (
    `${head}<text x="${cx}" y="${cy}" fill="${spec.color}" font-family="Inter, system-ui, sans-serif" ` +
    `font-size="${fs}" font-weight="700" letter-spacing="0.06em" text-anchor="middle" ` +
    `dominant-baseline="central"${rot}>${escXml(spec.text)}</text></svg>`
  );
};

/**
 * Estilo en línea (render) de la capa de marca de agua: imagen SVG de fondo
 * repetida una vez por alto de página, centrada y detrás del contenido
 * (z-index:-1). Cadena pura para `style="…"`; la cubre el self-test.
 */
export const watermarkStyle = (spec: WatermarkSpec): string => {
  const uri = `data:image/svg+xml,${encodeSvgForCss(watermarkSvg(spec))}`;
  return (
    'position:absolute;inset:0;z-index:-1;pointer-events:none;user-select:none;' +
    // Fuerza a imprimir el fondo aunque el diálogo tenga «gráficos de fondo» apagado.
    'print-color-adjust:exact;-webkit-print-color-adjust:exact;' +
    `opacity:${spec.opacity};` +
    `background-image:url('${uri}');` +
    'background-repeat:repeat-y;background-position:center top;' +
    `background-size:100% ${WATERMARK_TILE_H}px;`
  );
};

/**
 * HTML completo de la capa de marca de agua para insertar/regenerar en el folio:
 * `<div data-sytue-watermark>` con los ajustes en `data-*` (fuente de verdad) y el
 * render horneado en `style`. `aria-hidden` + `contenteditable=false`: decorativa
 * e inerte. Puro y exportado para el self-test.
 */
export const watermarkHtml = (spec: WatermarkSpec): string => {
  const attrs =
    `data-sytue-watermark data-kind="${spec.kind}" ` +
    `data-color="${escAttr(spec.color)}" data-opacity="${spec.opacity}" ` +
    `data-size="${spec.size}" data-diagonal="${spec.diagonal ? '1' : '0'}"` +
    (spec.kind === 'text'
      ? ` data-text="${escAttr(spec.text)}"`
      : ` data-image="${escAttr(spec.image ?? '')}"`);
  return `<div ${attrs} contenteditable="false" aria-hidden="true" style="${escAttr(
    watermarkStyle(spec),
  )}"></div>`;
};

/* ==================================================================== */
/* Borde de página                                                       */
/* ==================================================================== */

export type PageBorderStyleId = 'solid' | 'dashed' | 'dotted' | 'double';
const BORDER_STYLES: readonly PageBorderStyleId[] = ['solid', 'dashed', 'dotted', 'double'];

/** Ajustes del borde de página (fuente de verdad: atributos `data-*`). */
export interface PageBorderSpec {
  style: PageBorderStyleId;
  /** Grosor del trazo en px. */
  width: number;
  color: string;
}

export const DEFAULT_BORDER_COLOR = '#475569';

/** Presets sobrios (arte simple sin clip-art): estilo + grosor + color. */
export const PAGE_BORDER_PRESETS: readonly {
  id: string;
  labelKey: DocsKey;
  spec: PageBorderSpec;
}[] = [
  {
    id: 'fina',
    labelKey: 'border.preset.thinLine',
    spec: { style: 'solid', width: 1, color: '#475569' },
  },
  {
    id: 'gruesa',
    labelKey: 'border.preset.thickLine',
    spec: { style: 'solid', width: 3, color: '#0A0A0A' },
  },
  {
    id: 'doble',
    labelKey: 'border.style.double',
    spec: { style: 'double', width: 4, color: '#475569' },
  },
  {
    id: 'discontinua',
    labelKey: 'border.style.dashed',
    spec: { style: 'dashed', width: 2, color: '#475569' },
  },
  {
    id: 'punteada',
    labelKey: 'border.style.dotted',
    spec: { style: 'dotted', width: 2, color: '#94A3B8' },
  },
];

/** Sanea unos ajustes a un PageBorderSpec, o `null` si el grosor es ≤ 0 (sin borde). */
export const sanitizePageBorderSpec = (raw: {
  style?: unknown;
  width?: unknown;
  color?: unknown;
}): PageBorderSpec | null => {
  const width = Math.round(clamp(num(raw.width, 0), 0, 12));
  if (width <= 0) return null;
  const style = BORDER_STYLES.includes(raw.style as PageBorderStyleId)
    ? (raw.style as PageBorderStyleId)
    : 'solid';
  return { style, width, color: safeHex(raw.color, DEFAULT_BORDER_COLOR) };
};

/**
 * Estilo en línea (render) del borde: recuadro absoluto embutido en el margen
 * (a media distancia del borde del papel, siguiendo el margen VIVO
 * var(--doc-page-margin); reserva 96px para el export autocontenido), sin
 * capturar ratón. Cadena pura para `style="…"`; la cubre el self-test.
 *
 * ponytail: un ÚNICO recuadro sobre el folio continuo, no un marco por página
 * (sin arista superior/inferior en los saltos de un documento multipágina);
 * coherente con la numeración y el encabezado/pie, que ya asumen folio continuo.
 * Marco real por página cuando el lienzo tenga cortes de verdad (fase 2).
 */
export const pageBorderStyle = (spec: PageBorderSpec): string => {
  const inset = 'calc(var(--doc-page-margin, 96px) / 2)';
  return (
    'position:absolute;pointer-events:none;z-index:0;' +
    `top:${inset};left:${inset};right:${inset};bottom:${inset};` +
    `border:${spec.width}px ${spec.style} ${spec.color};`
  );
};

/**
 * HTML completo de la capa de borde de página para insertar/regenerar en el
 * folio: `<div data-sytue-page-border>` con los ajustes en `data-*` y el render en
 * `style`. Inerte y decorativo. Puro y exportado para el self-test.
 */
export const pageBorderHtml = (spec: PageBorderSpec): string =>
  `<div data-sytue-page-border data-style="${spec.style}" data-width="${spec.width}" ` +
  `data-color="${escAttr(spec.color)}" contenteditable="false" aria-hidden="true" ` +
  `style="${escAttr(pageBorderStyle(spec))}"></div>`;
