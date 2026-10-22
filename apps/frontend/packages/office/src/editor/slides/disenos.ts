/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica PURA de los diseños de una presentación: dentro del patrón (`master.layouts`),
 * cada uno con su nombre y sus marcadores de posición —cajas de texto con su muestra—, de
 * los que parte una diapositiva nueva; y cada diapositiva dice cuál lleva (`layout`). Sin
 * Preact ni DOM: se prueba con bun (test/slides-disenos.test.ts).
 *
 * Frontera de confianza: el modelo lo escriben otras personas y otras versiones, así que
 * `sanitizeLayouts` rehace la lista al cargar y al editar: solo cajas de texto, dentro del
 * lienzo y hasta unos topes. Los mismos que `slides_disenos.go` del servicio.
 */
import type { SlideBox, SlideConnector, SlideLayout } from '../types';
import { sanitizeConnectors } from './connector';

export const MAX_LAYOUTS = 24;
export const MAX_PLACEHOLDERS = 12;
export const MAX_LAYOUT_NAME = 60;
/** Techo de la muestra de un marcador (caracteres de HTML). */
const MAX_SAMPLE_CHARS = 2000;
/** Techos de las formas y las líneas de un diseño (maxDecorDiseno y maxLineasDiseno). */
export const MAX_LAYOUT_DECOR = 200;
export const MAX_LAYOUT_CONNECTORS = 100;

/** Las formas de un diseño: las cajas bien formadas, hasta el tope. Se pintan como las del
 *  patrón (el HTML pasa por el saneado al pintar). */
function sanitizeDecor(raw: unknown): SlideBox[] {
  if (!Array.isArray(raw)) return [];
  const out: SlideBox[] = [];
  for (const b of raw) {
    if (out.length >= MAX_LAYOUT_DECOR) break;
    if (!b || typeof b !== 'object') continue;
    const bo = b as Record<string, unknown>;
    if (typeof bo['id'] !== 'string' || typeof bo['html'] !== 'string') continue;
    if (!['x', 'y', 'w', 'h'].every((k) => typeof bo[k] === 'number' && Number.isFinite(bo[k])))
      continue;
    out.push(b as SlideBox);
  }
  return out;
}

const clampPct = (v: unknown): number => {
  const n = typeof v === 'number' && Number.isFinite(v) ? v : 0;
  return Math.min(100, Math.max(0, Math.round(n * 1000) / 1000));
};

/** Rehace la lista de diseños: ids válidos y únicos, nombre acotado y, de cada uno, solo
 *  las cajas de texto (los marcadores), hasta el tope y dentro del lienzo. */
export function sanitizeLayouts(raw: unknown): SlideLayout[] {
  if (!Array.isArray(raw)) return [];
  const out: SlideLayout[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    if (out.length >= MAX_LAYOUTS) break;
    if (!item || typeof item !== 'object') continue;
    const o = item as Record<string, unknown>;
    const id = typeof o['id'] === 'string' && /^[A-Za-z0-9_-]{1,40}$/.test(o['id']) ? o['id'] : '';
    if (!id || seen.has(id)) continue;
    seen.add(id);
    const name =
      typeof o['name'] === 'string'
        ? Array.from(o['name'].trim()).slice(0, MAX_LAYOUT_NAME).join('')
        : '';
    const boxes: SlideBox[] = [];
    for (const b of Array.isArray(o['boxes']) ? o['boxes'] : []) {
      if (boxes.length >= MAX_PLACEHOLDERS) break;
      if (!b || typeof b !== 'object') continue;
      const bo = b as Record<string, unknown>;
      if (bo['kind'] !== undefined && bo['kind'] !== 'text') continue;
      boxes.push({
        id:
          typeof bo['id'] === 'string' && bo['id'] !== ''
            ? bo['id'].slice(0, 60)
            : `${id}-${boxes.length + 1}`,
        x: clampPct(bo['x']),
        y: clampPct(bo['y']),
        w: clampPct(bo['w']),
        h: clampPct(bo['h']),
        html: typeof bo['html'] === 'string' ? bo['html'].slice(0, MAX_SAMPLE_CHARS) : '',
      });
    }
    const layout: SlideLayout = { id, name, boxes };
    const decor = sanitizeDecor(o['decor']);
    if (decor.length > 0) layout.decor = decor;
    const connectors: SlideConnector[] = sanitizeConnectors(o['connectors'], new Set()).slice(
      0,
      MAX_LAYOUT_CONNECTORS,
    );
    if (connectors.length > 0) layout.connectors = connectors;
    if (o['hideMaster'] === true) layout.hideMaster = true;
    out.push(layout);
  }
  return out;
}

/** Los diseños vigentes: los del patrón (saneados) o, si el modelo no los toca, los de
 *  serie que da `builtin`. */
export function layoutsOf(
  layouts: readonly SlideLayout[] | undefined,
  builtin: () => SlideLayout[],
): SlideLayout[] {
  return layouts === undefined ? builtin() : sanitizeLayouts(layouts);
}

/** Id libre para un diseño nuevo: `diseno-1`, `diseno-2`… */
export function nextLayoutId(layouts: readonly SlideLayout[]): string {
  const used = new Set(layouts.map((l) => l.id));
  for (let n = 1; ; n++) if (!used.has(`diseno-${n}`)) return `diseno-${n}`;
}

/** Diseño nuevo al final de la lista (con un título y un cuerpo de partida si no se dan
 *  cajas); `null` si ya hay `MAX_LAYOUTS`. */
export function addLayout(
  layouts: readonly SlideLayout[],
  name: string,
  boxes: SlideBox[],
  extra: Pick<SlideLayout, 'decor' | 'connectors' | 'hideMaster'> = {},
): { layouts: SlideLayout[]; id: string } | null {
  const list = sanitizeLayouts(layouts);
  if (list.length >= MAX_LAYOUTS) return null;
  const id = nextLayoutId(list);
  const [created] = sanitizeLayouts([{ ...extra, id, name, boxes }]);
  return created ? { layouts: [...list, created], id } : null;
}

/** Copia de un diseño con otro nombre y otras cajas (ids nuevos). */
export function duplicateLayout(
  layouts: readonly SlideLayout[],
  id: string,
  name: string,
  newBoxId: () => string,
): { layouts: SlideLayout[]; id: string } | null {
  const src = layouts.find((l) => l.id === id);
  return src
    ? addLayout(
        layouts,
        name,
        src.boxes.map((b) => ({ ...b, id: newBoxId() })),
        src, // sus formas, sus líneas y si tapa el patrón
      )
    : null;
}

/** Cambia el nombre o los marcadores de un diseño. */
export function patchLayout(
  layouts: readonly SlideLayout[],
  id: string,
  patch: Partial<Pick<SlideLayout, 'name' | 'boxes'>>,
): SlideLayout[] {
  return sanitizeLayouts(layouts.map((l) => (l.id === id ? { ...l, ...patch } : l)));
}

/** Quita un diseño. */
export function removeLayout(layouts: readonly SlideLayout[], id: string): SlideLayout[] {
  return sanitizeLayouts(layouts.filter((l) => l.id !== id));
}

/** Las cajas de una diapositiva que nace con este diseño: sus marcadores, con ids nuevos. */
export function boxesFromLayout(layout: SlideLayout, newBoxId: () => string): SlideBox[] {
  return layout.boxes.map((b) => ({ ...b, id: newBoxId() }));
}

/** Reflujo de un diseño sobre las cajas de una diapositiva: el TEXTO entra por orden en los
 *  marcadores del diseño; imágenes, formas y demás se conservan tal cual (cambiar de
 *  diseño no puede destruirlas). */
export function mergeIntoLayout(
  layout: SlideLayout,
  boxes: SlideBox[],
  newBoxId: () => string,
): SlideBox[] {
  const texts = boxes.filter((b) => !b.kind || b.kind === 'text');
  const rest = boxes.filter((b) => b.kind !== undefined && b.kind !== 'text');
  return [
    ...boxesFromLayout(layout, newBoxId).map((tb, i) => {
      const old = texts[i];
      return old ? { ...tb, html: old.html } : tb;
    }),
    ...rest,
  ];
}

/** Diseño de una diapositiva nueva: «título y cuerpo» si la presentación lo tiene; si no,
 *  el de la diapositiva desde la que se crea (como PowerPoint) y, si no lo tiene, el
 *  primero con título y cuerpo (o el primero, sea cual sea). `''` si no hay diseños. */
export function defaultNewLayoutId(
  layouts: readonly SlideLayout[],
  activeLayout: string | undefined,
): string {
  const ids = new Set(layouts.map((l) => l.id));
  if (ids.has('titulo-cuerpo')) return 'titulo-cuerpo';
  if (activeLayout && ids.has(activeLayout)) return activeLayout;
  return (layouts.find((l) => l.boxes.length >= 2) ?? layouts[0])?.id ?? '';
}
