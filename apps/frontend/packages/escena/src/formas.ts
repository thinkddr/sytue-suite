/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Geometría PURA de las formas poligonales de la escena (movida desde
 * `packages/office/src/editor/slides/shapes.ts`, que ahora la importa de aquí). Dado
 * el tamaño en px de la caja (0..w × 0..h), devuelve los vértices de las formas
 * POLIGONALES (triángulo, rombo, pentágono, hexágono, estrella, bocadillo) y las
 * medidas del cilindro (dos tapas elípticas + cuerpo). Sin DOM. Las formas del
 * núcleo (rect/ellipse/line/arrow) las pinta el renderizador con su primitiva
 * SVG propia. Pruebas: formas.test.ts (y packages/office/test/slides-shapes.test.ts).
 */
/** Punto en el espacio px de la caja. */
export type ShapePoint = readonly [number, number];

/** Margen interior para que el trazo (grosor 2) no se recorte en el borde. */
const INSET = 2;

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));
const round2 = (n: number): number => Math.round(n * 100) / 100;

/** Vértices de un polígono regular de `n` lados inscrito en la elipse (rx/ry)
 *  centrada en (cx,cy), desde el ángulo `start` (rad), sentido horario. */
const regular = (
  cx: number,
  cy: number,
  rx: number,
  ry: number,
  n: number,
  start: number,
): ShapePoint[] =>
  Array.from({ length: n }, (_, k) => {
    const a = start + (k * 2 * Math.PI) / n;
    return [cx + rx * Math.cos(a), cy + ry * Math.sin(a)] as ShapePoint;
  });

/** Vértices de una estrella de `points` puntas: radios alternos (exterior 1,
 *  interior `inner`) recorridos cada 180/points grados desde `start`. */
const star = (
  cx: number,
  cy: number,
  rx: number,
  ry: number,
  points: number,
  start: number,
  inner: number,
): ShapePoint[] =>
  Array.from({ length: points * 2 }, (_, k) => {
    const a = start + (k * Math.PI) / points;
    const rr = k % 2 === 0 ? 1 : inner;
    return [cx + rx * rr * Math.cos(a), cy + ry * rr * Math.sin(a)] as ShapePoint;
  });

/** Bocadillo (callout): cuerpo rectangular con una cola triangular abajo a la
 *  izquierda. Polígono de 7 vértices (la cola sustituye parte del borde inferior). */
const callout = (w: number, h: number, p: number): ShapePoint[] => {
  const x0 = p;
  const x1 = w - p;
  const y0 = p;
  const y1 = h - p;
  const tail = clamp(h * 0.22, 4, Math.max(4, (y1 - y0) * 0.45)); // alto de la cola
  const yb = y1 - tail; // base inferior del cuerpo (encima de la cola)
  const bx = x0 + w * 0.16; // arranque izquierdo de la base de la cola
  const bw = Math.min(w * 0.16, x1 - bx); // ancho de la base de la cola
  const tip = clamp(x0 + w * 0.06, x0, x1); // punta de la cola (abajo-izquierda)
  return [
    [x0, y0],
    [x1, y0],
    [x1, yb],
    [bx + bw, yb],
    [tip, y1],
    [bx, yb],
    [x0, yb],
  ];
};

/** Vértices (px) de una forma POLIGONAL inscrita en la caja w×h; `[]` si la forma
 *  no es poligonal (rect/ellipse/line/arrow/cylinder → las pinta el render con su
 *  primitiva). Pura y exportada para el self-test (bun test). */
export function polygonPoints(shape: string, w: number, h: number): ShapePoint[] {
  const p = INSET;
  const cx = w / 2;
  const cy = h / 2;
  const rx = Math.max(0.5, cx - p);
  const ry = Math.max(0.5, cy - p);
  const x0 = p;
  const x1 = w - p;
  const y0 = p;
  const y1 = h - p;
  switch (shape) {
    case 'triangle':
      return [
        [cx, y0],
        [x1, y1],
        [x0, y1],
      ];
    case 'diamond':
      return [
        [cx, y0],
        [x1, cy],
        [cx, y1],
        [x0, cy],
      ];
    case 'pentagon':
      return regular(cx, cy, rx, ry, 5, -Math.PI / 2); // punta arriba
    case 'hexagon':
      return regular(cx, cy, rx, ry, 6, 0); // puntas izquierda/derecha (horizontal)
    case 'star':
      return star(cx, cy, rx, ry, 5, -Math.PI / 2, 0.42);
    case 'callout':
      return callout(w, h, p);
    default:
      return [];
  }
}

/** Cadena `points` de un `<polygon>` SVG a partir de los vértices. Pura. */
export const polygonAttr = (pts: ShapePoint[]): string =>
  pts.map((pt) => `${round2(pt[0])},${round2(pt[1])}`).join(' ');

/* ---------- Cilindro (dos tapas elípticas + cuerpo) ---------- */

export interface CylinderDims {
  /** Radio horizontal de las tapas (media anchura útil). */
  rx: number;
  /** Radio vertical (semialto) de las tapas elípticas. */
  ry: number;
  /** Centro horizontal. */
  cx: number;
  /** Y del centro de la tapa superior y de la inferior (yt < yb). */
  yt: number;
  yb: number;
}

/** Medidas del cilindro inscrito en la caja w×h. Pura y exportada para bun test. */
export function cylinderDims(w: number, h: number): CylinderDims {
  const p = INSET;
  const rx = Math.max(0.5, w / 2 - p);
  const ry = clamp(h * 0.14, 3, Math.max(3, h / 2 - p));
  return { rx, ry, cx: w / 2, yt: p + ry, yb: h - p - ry };
}

/** Silueta cerrada del cilindro (cuerpo + tapa superior por detrás + tapa
 *  inferior por delante). Todos los arcos con sweep 0 (ver nota). Pura + test.
 *  El sweep 0 dibuja el arco inferior abombando HACIA ABAJO (izq→der) y el
 *  superior HACIA ARRIBA (der→izq): la silueta correcta de un cilindro. */
export function cylinderBodyPath(d: CylinderDims): string {
  const xl = round2(d.cx - d.rx);
  const xr = round2(d.cx + d.rx);
  const yt = round2(d.yt);
  const yb = round2(d.yb);
  const rx = round2(d.rx);
  const ry = round2(d.ry);
  return (
    `M ${xl} ${yt} V ${yb} A ${rx} ${ry} 0 0 0 ${xr} ${yb}` +
    ` V ${yt} A ${rx} ${ry} 0 0 0 ${xl} ${yt} Z`
  );
}

/** Costura del cilindro: arco FRONTAL de la tapa superior (izq→der, abomba hacia
 *  abajo). Solo trazo (fill none). Pura + test. */
export function cylinderSeamPath(d: CylinderDims): string {
  const xl = round2(d.cx - d.rx);
  const xr = round2(d.cx + d.rx);
  const yt = round2(d.yt);
  return `M ${xl} ${yt} A ${round2(d.rx)} ${round2(d.ry)} 0 0 0 ${xr} ${yt}`;
}
