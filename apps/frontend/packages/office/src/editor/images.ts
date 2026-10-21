/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Manejo avanzado de IMAGEN del editor de documentos — AYUDAS PURAS (sin DOM).
 * El HTML del folio ES el modelo (§6.5): cada imagen es un ÚNICO `<img>` cuyo
 * estado (tamaño, ajuste de texto, anclaje, recorte, alt) vive en atributos
 * `data-*` (fuente de verdad) y se refleja en su `style` en línea (render). Aquí
 * solo la lógica no trivial y testeable: (de)serializar ese modelo y CALCULAR el
 * `style` —el float del ajuste y el recorte visible—; el pegamento con el folio
 * (seleccionar, tiradores, aplicar) vive en canvas/imageDom.ts y
 * canvas/ImageOverlay.tsx. Mismo patrón que smartart.ts / formfields.ts.
 *
 * Recorte SIN contenedor: `clip-path: inset(...)` oculta las bandas recortadas y
 * unos márgenes NEGATIVOS del mismo tamaño recuperan su espacio de maqueta, de
 * modo que el texto y los vecinos se ajustan al recorte y no al tamaño completo
 * (un solo elemento ⇒ retrocompatible con las imágenes inline ya existentes).
 * Ejecutar el self-test: cd apps/frontend && bun test packages/office/test/docs-images.test.ts
 */

import type { DocsKey } from '../i18n';

/** Ajuste de texto de la imagen (paridad Word). */
export type ImageWrap = 'inline' | 'square' | 'tight' | 'topBottom' | 'behind' | 'front';
/** Alineación de la imagen flotante (float) o en bloque (arriba y abajo). */
export type ImageAlign = 'left' | 'center' | 'right';

/** Recorte por lado como FRACCIÓN [0,~0.9) del lado correspondiente. */
export interface ImageCrop {
  t: number;
  r: number;
  b: number;
  l: number;
}

/**
 * Modelo de una imagen gestionada. `w`/`h` son el tamaño de maqueta de la imagen
 * COMPLETA (antes de recortar); `x`/`y` (px, relativos al folio) solo aplican al
 * anclaje flotante detrás/delante del texto. `src` no se persiste (se lee del
 * propio `<img>`): lo usa el ajuste «ajustado» para el contorno por alfa.
 */
export interface ImageModel {
  wrap: ImageWrap;
  align: ImageAlign;
  crop: ImageCrop;
  alt: string;
  w?: number;
  h?: number;
  x?: number;
  y?: number;
  src?: string;
}

/** Ajustes de texto ofrecidos por la UI (fuente única id → CLAVE del catálogo).
 *  Guarda claves y no rótulos: un array de módulo con texto se congelaría con el
 *  idioma de la primera carga; la `t` de cada componente lo resuelve al pintar. */
export const IMAGE_WRAPS: readonly { id: ImageWrap; labelKey: DocsKey }[] = [
  { id: 'inline', labelKey: 'image.wrap.inline' },
  { id: 'square', labelKey: 'image.wrap.square' },
  { id: 'tight', labelKey: 'image.wrap.tight' },
  { id: 'topBottom', labelKey: 'image.wrap.topBottom' },
  { id: 'behind', labelKey: 'image.wrap.behind' },
  { id: 'front', labelKey: 'image.wrap.front' },
];

const WRAP_IDS = IMAGE_WRAPS.map((w) => w.id);
const ALIGN_IDS: readonly ImageAlign[] = ['left', 'center', 'right'];

/** Ajuste flotante en la corriente del texto (float): admite alineación i/d. */
export const isFloatWrap = (wrap: ImageWrap): boolean => wrap === 'square' || wrap === 'tight';
/** Ajuste absoluto (fuera de la corriente): posicionable con x/y y z-index. */
export const isAbsoluteWrap = (wrap: ImageWrap): boolean => wrap === 'behind' || wrap === 'front';

/* ---------- saneado / parseo (frontera de datos: modelo viejo/corrupto no revienta) ---------- */

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));
/** Redondeo a centésima (px compactos, mismo criterio que slides/smartart). */
const r2 = (n: number): number => Math.round(n * 100) / 100;
/** Redondeo a milésima (fracciones de recorte). */
const r3 = (n: number): number => Math.round(n * 1000) / 1000;

/** Número finito de una cadena, o `undefined` (para campos opcionales). */
const num = (raw: string | null | undefined): number | undefined => {
  if (raw == null || raw === '') return undefined;
  const n = Number(raw);
  return Number.isFinite(n) ? n : undefined;
};

export const sanitizeWrap = (raw: string | null | undefined): ImageWrap =>
  WRAP_IDS.includes(raw as ImageWrap) ? (raw as ImageWrap) : 'inline';

export const sanitizeAlign = (raw: string | null | undefined): ImageAlign =>
  ALIGN_IDS.includes(raw as ImageAlign) ? (raw as ImageAlign) : 'left';

/**
 * Sanea un recorte: cada lado en [0, 0.9] y, por eje, la suma acotada a 0.9 (deja
 * ≥10% visible para que la imagen nunca se recorte a nada). Puro y exportado para
 * poder validar entradas de la UI (steppers de recorte).
 */
export const clampCrop = (c: Partial<ImageCrop> | null | undefined): ImageCrop => {
  const side = (v: unknown): number => clamp(Number(v) || 0, 0, 0.9);
  let t = side(c?.t);
  let r = side(c?.r);
  let b = side(c?.b);
  let l = side(c?.l);
  // Acota por eje: si t+b (o l+r) pasa de 0.9, escala ambos proporcionalmente.
  const axis = (a: number, z: number): [number, number] => {
    const s = a + z;
    return s > 0.9 ? [(a * 0.9) / s, (z * 0.9) / s] : [a, z];
  };
  [t, b] = axis(t, b);
  [l, r] = axis(l, r);
  return { t: r3(t), r: r3(r), b: r3(b), l: r3(l) };
};

const hasCrop = (c: ImageCrop): boolean => c.t > 0 || c.r > 0 || c.b > 0 || c.l > 0;

/** Parsea «t,r,b,l» → recorte saneado; ausente/ inválido → sin recorte. */
const parseCrop = (raw: string | null | undefined): ImageCrop => {
  const p = (raw ?? '').split(',');
  return clampCrop({ t: Number(p[0]), r: Number(p[1]), b: Number(p[2]), l: Number(p[3]) });
};

/**
 * Lee el modelo de una imagen desde un getter de atributos (en el DOM:
 * `(n) => img.getAttribute(n)`). `alt`/`src` son atributos nativos; el resto
 * `data-*`. Tolerante: todo campo ausente cae a su valor por defecto.
 */
export const parseImageModel = (get: (name: string) => string | null | undefined): ImageModel => {
  const wrap = sanitizeWrap(get('data-wrap'));
  const model: ImageModel = {
    wrap,
    align: sanitizeAlign(get('data-align')),
    crop: parseCrop(get('data-crop')),
    alt: get('alt') ?? '',
  };
  // exactOptionalPropertyTypes: los opcionales se OMITEN si no vienen (no `= undefined`).
  const w = num(get('data-w'));
  const h = num(get('data-h'));
  const x = num(get('data-x'));
  const y = num(get('data-y'));
  const src = get('src');
  if (w !== undefined) model.w = w;
  if (h !== undefined) model.h = h;
  if (isAbsoluteWrap(wrap) && x !== undefined) model.x = x;
  if (isAbsoluteWrap(wrap) && y !== undefined) model.y = y;
  if (src) model.src = src;
  return model;
};

/**
 * Atributos `data-*` a escribir para un modelo (JSON mínimo: se omite lo que está
 * en su valor por defecto). NO incluye `alt`/`src` (atributos nativos, los fija el
 * llamante). Inversa de parseImageModel para los campos que persiste.
 */
export const imageDataAttrs = (m: ImageModel): Record<string, string> => {
  const a: Record<string, string> = {};
  if (m.wrap !== 'inline') a['data-wrap'] = m.wrap;
  if (m.align !== 'left') a['data-align'] = m.align;
  if (m.w !== undefined) a['data-w'] = String(r2(m.w));
  if (m.h !== undefined) a['data-h'] = String(r2(m.h));
  if (hasCrop(m.crop)) a['data-crop'] = `${m.crop.t},${m.crop.r},${m.crop.b},${m.crop.l}`;
  if (isAbsoluteWrap(m.wrap)) {
    if (m.x !== undefined) a['data-x'] = String(r2(m.x));
    if (m.y !== undefined) a['data-y'] = String(r2(m.y));
  }
  return a;
};

/* ---------- cálculo del style en línea (el render de la imagen) ---------- */

/** Separación (px) entre una imagen flotante/en bloque y el texto que la rodea. */
const GAP = 14;
const px = (n: number): string => `${r2(n)}px`;

/**
 * `style` en línea de la imagen para un modelo (resize + ajuste + recorte +
 * anclaje). Función PURA y clave del feature: la cubre el self-test. Devuelve un
 * mapa propiedad→valor (camelCase); styleCssText lo serializa para el atributo.
 *
 * Recorte: `clip-path: inset(top right bottom left)` oculta las bandas y unos
 * márgenes NEGATIVOS del mismo tamaño recuperan su espacio, de modo que el texto
 * se ciñe al recorte. La aritmética `separación − banda_oculta` deja SIEMPRE la
 * separación pedida entre el borde visible y el texto (ver comentario del módulo).
 */
export const computeImageStyle = (m: ImageModel): Record<string, string> => {
  const style: Record<string, string> = {};
  const { w, h } = m;

  // Tamaño de la imagen completa (pre-recorte). En línea se acota al ancho útil
  // para no desbordar el folio; flotante/absoluta respetan el tamaño (la UI acota
  // los tiradores). height:auto de editor.css se anula al fijar height.
  if (w !== undefined) style.width = px(w);
  if (h !== undefined) style.height = px(h);
  style.maxWidth = m.wrap === 'inline' ? '100%' : 'none';

  // Recorte visible + bandas ocultas (px) para recuperar su espacio con márgenes.
  const band = { t: 0, r: 0, b: 0, l: 0 };
  if (hasCrop(m.crop) && w !== undefined && h !== undefined) {
    band.t = m.crop.t * h;
    band.r = m.crop.r * w;
    band.b = m.crop.b * h;
    band.l = m.crop.l * w;
    style.clipPath = `inset(${px(band.t)} ${px(band.r)} ${px(band.b)} ${px(band.l)})`;
  }

  // Separación del modo (positiva) por lado; el recorte la ajusta después.
  const gap = { t: 0, r: 0, b: 0, l: 0 };
  switch (m.wrap) {
    case 'square':
    case 'tight': {
      const left = m.align !== 'right'; // imagen a la izquierda, texto a su derecha
      style.float = left ? 'left' : 'right';
      gap.b = GAP;
      if (left) gap.r = GAP;
      else gap.l = GAP;
      if (m.wrap === 'tight' && m.src) {
        // Contorno rectangular por CSS Shapes que RESPETA el recorte (mismo inset
        // que el clip-path) + shape-margin ceñido. shape-outside exige un float con
        // dimensiones (de ahí el requisito de src/tamaño para activarlo).
        // ponytail: contorno por alfa real = shape-outside:url(src), pero incrustar
        // el dataURL en el estilo duplicaría el peso de la imagen en cada autosave;
        // upgrade a url(src) si se pide paridad estricta de «ajustado».
        style.shapeOutside = `inset(${px(band.t)} ${px(band.r)} ${px(band.b)} ${px(band.l)})`;
        style.shapeMargin = `${GAP}px`;
      }
      break;
    }
    case 'topBottom':
      style.display = 'block';
      style.clear = 'both';
      gap.t = GAP;
      gap.b = GAP;
      // Alineación por márgenes auto (bloque); el recorte horizontal no se recupera
      // en este modo (bloque centrado ⇒ auto), solo el vertical.
      style.marginLeft = m.align === 'left' ? '0' : 'auto';
      style.marginRight = m.align === 'right' ? '0' : 'auto';
      break;
    case 'behind':
    case 'front':
      style.position = 'absolute';
      style.left = px(m.x ?? 0);
      style.top = px(m.y ?? 0);
      // Detrás: bajo el texto pero sobre el fondo del folio. Delante: sobre todo.
      style.zIndex = m.wrap === 'behind' ? '-1' : '3';
      break;
    case 'inline':
    default:
      style.display = 'inline-block';
      style.verticalAlign = 'bottom';
      break;
  }

  // Márgenes finales = separación − banda oculta (los ya fijados con auto se respetan).
  const apply = (side: keyof typeof gap, prop: string): void => {
    if (style[prop] !== undefined) return;
    const val = r2(gap[side] - band[side]);
    if (val !== 0) style[prop] = px(val);
  };
  apply('t', 'marginTop');
  apply('r', 'marginRight');
  apply('b', 'marginBottom');
  apply('l', 'marginLeft');

  return style;
};

const KEBAB = /[A-Z]/g;
/** Serializa un mapa de estilo (camelCase) al texto del atributo `style`. */
export const styleCssText = (style: Record<string, string>): string =>
  Object.entries(style)
    .map(([k, v]) => `${k.replace(KEBAB, (ch) => `-${ch.toLowerCase()}`)}:${v}`)
    .join(';');
