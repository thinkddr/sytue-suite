/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Trazado PURO de los conectores (movido desde `packages/office/src/editor/slides/
 * connector.ts`, que ahora lo importa de aquí). Un conector une dos EXTREMOS
 * —una forma (rect, por su borde) o un punto libre— con un trazado recto, en
 * ángulo (codo) o curvo, con punta de flecha opcional en cada extremo. Todo en
 * un espacio de coordenadas abstracto (px): el llamante convierte lo suyo (% en
 * Diapos, px en la escena) y pasa rects ya resueltos. Sin DOM.
 *
 * Anclaje 'auto': el extremo sale por el borde de su forma que MIRA al otro
 * extremo (intersección del rayo centro→objetivo con el rectángulo), así el
 * conector se recalcula solo al mover las formas. El objetivo de cada extremo es
 * el centro/punto del OTRO (no circular). Todo O(1) por conector.
 */

/** Lado de anclaje de un extremo al borde de una forma. 'auto' = el borde que
 *  mira al otro extremo (se recalcula al mover la forma). */
export type ConnectorAnchor = 'auto' | 'top' | 'right' | 'bottom' | 'left';
/** Trazado de un conector: recto, en ángulo (codo) o curvo. */
export type ConnectorKind = 'straight' | 'elbow' | 'curved';

export interface CPoint {
  x: number;
  y: number;
}
export interface CRect {
  x: number;
  y: number;
  w: number;
  h: number;
}
/** Extremo ya resuelto a px: anclado a un rect (con lado) o punto libre. */
export type ConnEnd = { rect: CRect; anchor: ConnectorAnchor } | { point: CPoint };

/** Extremo del trazado: posición + ángulo (rad) de salida (hacia donde apunta la
 *  punta de flecha, si la hay). */
export interface EndCap {
  x: number;
  y: number;
  angle: number;
}
export interface ConnectorRoute {
  /** Path SVG principal (en px del escenario). */
  d: string;
  start: EndCap;
  end: EndCap;
}

const round2 = (n: number): number => Math.round(n * 100) / 100;

const center = (r: CRect): CPoint => ({ x: r.x + r.w / 2, y: r.y + r.h / 2 });

/** Punto representativo de un extremo (para el anclaje 'auto' del OTRO): el
 *  centro del rect o el propio punto libre. */
const targetOf = (e: ConnEnd): CPoint => ('rect' in e ? center(e.rect) : e.point);

const unit = (dx: number, dy: number): CPoint => {
  const l = Math.hypot(dx, dy) || 1;
  return { x: dx / l, y: dy / l };
};

const angleOf = (from: CPoint, to: CPoint): number => Math.atan2(to.y - from.y, to.x - from.x);

/** Salida de un rayo desde el centro del rect hacia `to`, sobre su borde. Pura. */
export function rayExit(r: CRect, to: CPoint): CPoint {
  const c = center(r);
  const dx = to.x - c.x;
  const dy = to.y - c.y;
  if (dx === 0 && dy === 0) return { x: r.x + r.w, y: c.y }; // objetivo en el centro: borde derecho
  const tx = dx !== 0 ? r.w / 2 / Math.abs(dx) : Infinity;
  const ty = dy !== 0 ? r.h / 2 / Math.abs(dy) : Infinity;
  const t = Math.min(tx, ty);
  return { x: c.x + dx * t, y: c.y + dy * t };
}

const sideMid = (r: CRect, side: ConnectorAnchor): CPoint => {
  switch (side) {
    case 'top':
      return { x: r.x + r.w / 2, y: r.y };
    case 'bottom':
      return { x: r.x + r.w / 2, y: r.y + r.h };
    case 'left':
      return { x: r.x, y: r.y + r.h / 2 };
    case 'right':
      return { x: r.x + r.w, y: r.y + r.h / 2 };
    default:
      return center(r);
  }
};

/** Punto de anclaje de un extremo mirando a `to` (para 'auto'). Pura + test. */
export function anchorPoint(e: ConnEnd, to: CPoint): CPoint {
  if (!('rect' in e)) return e.point;
  return e.anchor === 'auto' ? rayExit(e.rect, to) : sideMid(e.rect, e.anchor);
}

/** Dirección de salida (unitaria) en el anclaje: perpendicular al borde de la
 *  forma (para curvar) o hacia el otro extremo (punto libre). */
const outDir = (e: ConnEnd, at: CPoint, to: CPoint): CPoint => {
  if ('rect' in e) {
    const r = e.rect;
    if (Math.abs(at.x - r.x) < 0.5) return { x: -1, y: 0 };
    if (Math.abs(at.x - (r.x + r.w)) < 0.5) return { x: 1, y: 0 };
    if (Math.abs(at.y - r.y) < 0.5) return { x: 0, y: -1 };
    if (Math.abs(at.y - (r.y + r.h)) < 0.5) return { x: 0, y: 1 };
  }
  return unit(to.x - at.x, to.y - at.y);
};

const straight = (a: CPoint, b: CPoint): ConnectorRoute => ({
  d: `M ${round2(a.x)} ${round2(a.y)} L ${round2(b.x)} ${round2(b.y)}`,
  start: { x: a.x, y: a.y, angle: angleOf(b, a) },
  end: { x: b.x, y: b.y, angle: angleOf(a, b) },
});

const elbow = (a: CPoint, b: CPoint): ConnectorRoute => {
  let m1: CPoint;
  let m2: CPoint;
  if (Math.abs(b.x - a.x) >= Math.abs(b.y - a.y)) {
    const mx = (a.x + b.x) / 2; // codo horizontal-vertical-horizontal
    m1 = { x: mx, y: a.y };
    m2 = { x: mx, y: b.y };
  } else {
    const my = (a.y + b.y) / 2; // codo vertical-horizontal-vertical
    m1 = { x: a.x, y: my };
    m2 = { x: b.x, y: my };
  }
  return {
    d:
      `M ${round2(a.x)} ${round2(a.y)} L ${round2(m1.x)} ${round2(m1.y)}` +
      ` L ${round2(m2.x)} ${round2(m2.y)} L ${round2(b.x)} ${round2(b.y)}`,
    start: { x: a.x, y: a.y, angle: angleOf(m1, a) },
    end: { x: b.x, y: b.y, angle: angleOf(m2, b) },
  };
};

const curved = (a: CPoint, b: CPoint, da: CPoint, db: CPoint): ConnectorRoute => {
  const dist = Math.hypot(b.x - a.x, b.y - a.y);
  const k = Math.max(24, dist * 0.4); // tirón de los tiradores de la Bézier
  const c1 = { x: a.x + da.x * k, y: a.y + da.y * k };
  const c2 = { x: b.x + db.x * k, y: b.y + db.y * k };
  return {
    d:
      `M ${round2(a.x)} ${round2(a.y)} C ${round2(c1.x)} ${round2(c1.y)}` +
      ` ${round2(c2.x)} ${round2(c2.y)} ${round2(b.x)} ${round2(b.y)}`,
    start: { x: a.x, y: a.y, angle: angleOf(c1, a) },
    end: { x: b.x, y: b.y, angle: angleOf(c2, b) },
  };
};

/** Trazado de un conector entre dos extremos resueltos, según el tipo. Pura +
 *  testeada. `from`/`to` ya en px (el render convierte % → px). */
export function connectorRoute(from: ConnEnd, to: ConnEnd, kind: ConnectorKind): ConnectorRoute {
  const a = anchorPoint(from, targetOf(to));
  const b = anchorPoint(to, targetOf(from));
  if (kind === 'elbow') return elbow(a, b);
  if (kind === 'curved') return curved(a, b, outDir(from, a, b), outDir(to, b, a));
  return straight(a, b);
}

/** Punta de flecha (dos barbas) en (x,y) apuntando según `angle`. Pura. */
export function arrowHead(x: number, y: number, angle: number, size = 9): string {
  const spread = Math.PI / 7;
  const h1x = x - size * Math.cos(angle - spread);
  const h1y = y - size * Math.sin(angle - spread);
  const h2x = x - size * Math.cos(angle + spread);
  const h2y = y - size * Math.sin(angle + spread);
  return `M ${round2(h1x)} ${round2(h1y)} L ${round2(x)} ${round2(y)} L ${round2(h2x)} ${round2(h2y)}`;
}
