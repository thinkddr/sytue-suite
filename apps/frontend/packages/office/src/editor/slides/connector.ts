/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Conectores de diapositiva (fase Slides-Conectores): el saneado, remapeo y poda
 * del modelo de Diapos. El TRAZADO (rayos, codos, curvas, puntas) es geometría
 * común y vive en `@sytue/escena`; se reexporta aquí para no tocar a quien lo
 * importaba (SlidesCanvas, pruebas).
 */
import type { ConnectorAnchor, ConnectorEnd, ConnectorKind, SlideConnector } from '../types';

export { anchorPoint, arrowHead, connectorRoute, rayExit } from '@sytue/escena';
export type { ConnEnd, ConnectorRoute, CPoint, CRect, EndCap } from '@sytue/escena';

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));

/* ---------- Saneado / remapeo / poda (frontera de confianza + edición) ---------- */

const KINDS: ReadonlySet<ConnectorKind> = new Set<ConnectorKind>(['straight', 'elbow', 'curved']);
const SIDES: ReadonlySet<ConnectorAnchor> = new Set<ConnectorAnchor>([
  'auto',
  'top',
  'right',
  'bottom',
  'left',
]);
const HEX = /^#[0-9a-fA-F]{6}$/;
const pct = (v: unknown): number =>
  typeof v === 'number' && Number.isFinite(v) ? clamp(v, 0, 100) : 0;

const sanitizeEnd = (raw: unknown, validIds: Set<string>): ConnectorEnd => {
  const o = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>;
  const boxId = o['boxId'];
  if (typeof boxId === 'string' && validIds.has(boxId)) {
    const anchor = o['anchor'];
    return typeof anchor === 'string' && SIDES.has(anchor as ConnectorAnchor) && anchor !== 'auto'
      ? { boxId, anchor: anchor as ConnectorAnchor }
      : { boxId };
  }
  return { x: pct(o['x']), y: pct(o['y']) }; // sin caja válida = punto libre
};

/** Conectores saneados de una diapositiva (JSON crudo → válidos): tipo con
 *  reserva a 'straight', extremos anclados solo a cajas EXISTENTES (si no, punto
 *  libre), banderas/color opcionales. Pura y exportada para bun test. */
export function sanitizeConnectors(raw: unknown, validIds: Set<string>): SlideConnector[] {
  if (!Array.isArray(raw)) return [];
  const out: SlideConnector[] = [];
  raw.forEach((item, i) => {
    if (!item || typeof item !== 'object') return;
    const o = item as Record<string, unknown>;
    const kind = KINDS.has(o['kind'] as ConnectorKind) ? (o['kind'] as ConnectorKind) : 'straight';
    const id = typeof o['id'] === 'string' && o['id'] !== '' ? o['id'] : `c${i}`;
    const c: SlideConnector = {
      id,
      kind,
      from: sanitizeEnd(o['from'], validIds),
      to: sanitizeEnd(o['to'], validIds),
    };
    if (o['startArrow'] === true) c.startArrow = true;
    if (o['endArrow'] === true) c.endArrow = true;
    if (typeof o['stroke'] === 'string' && HEX.test(o['stroke'])) c.stroke = o['stroke'];
    out.push(c);
  });
  return out;
}

/** Remapea los ids de caja de los extremos (duplicar diapositiva: las cajas
 *  reciben ids nuevos). Los puntos libres quedan igual. Pura + test. */
export function remapConnectorIds(
  connectors: readonly SlideConnector[],
  idMap: Map<string, string>,
): SlideConnector[] {
  const remap = (e: ConnectorEnd): ConnectorEnd =>
    e.boxId !== undefined && idMap.has(e.boxId) ? { ...e, boxId: idMap.get(e.boxId)! } : e;
  return connectors.map((c) => ({ ...c, from: remap(c.from), to: remap(c.to) }));
}

/** Quita los conectores anclados a alguna caja ELIMINADA (borrar una forma borra
 *  sus conectores). Pura + test. */
export function pruneConnectors(
  connectors: readonly SlideConnector[],
  removed: Set<string>,
): SlideConnector[] {
  return connectors.filter(
    (c) =>
      !(c.from.boxId !== undefined && removed.has(c.from.boxId)) &&
      !(c.to.boxId !== undefined && removed.has(c.to.boxId)),
  );
}
