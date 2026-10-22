/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Geometría PURA de los diagramas SmartArt (fase Slides-SmartArt). Dados una
 * disposición, la lista de elementos y el tamaño de la caja en px intrínsecos
 * (0..vw × 0..vh, el mismo viewBox que usa el gráfico), reparte los nodos y
 * calcula los conectores. Sin Preact ni DOM: se testea aparte con bun.
 *
 * Layouts:
 *  - list      · tarjetas apiladas verticalmente (sin conectores).
 *  - process   · secuencia horizontal de cajas unidas por flechas.
 *  - cycle     · nodos (elipses) repartidos en un anillo con flechas en bucle.
 *  - hierarchy · organigrama de arriba abajo; la sangría `level` de cada item
 *                define el árbol (regla de esquema: un item cuelga del anterior
 *                con `level` estrictamente menor). Codos ortogonales sin punta.
 *
 * Todo es O(n): cada nodo se visita un número constante de veces.
 */
import type { SmartArtItem, SmartArtLayout } from '../types';

/** Nodo colocado: rectángulo (o elipse) en px de la caja + el índice del item. */
export interface SmartArtNode {
  /** Índice del item en `items` (para pintar su texto). */
  i: number;
  x: number;
  y: number;
  w: number;
  h: number;
  shape: 'rect' | 'ellipse';
}

/** Conector entre dos puntos. `arrow` = línea recta con punta (proceso/ciclo);
 *  `elbow` = codo ortogonal sin punta (jerarquía, estilo organigrama). */
export interface SmartArtConnector {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  kind: 'arrow' | 'elbow';
}

export interface SmartArtGeometry {
  nodes: SmartArtNode[];
  connectors: SmartArtConnector[];
}

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));

/** Distancia del centro de una elipse (radios rx/ry) a su borde en la dirección
 *  unitaria (ux,uy). Sirve para arrancar/parar las flechas del ciclo justo en el
 *  borde del nodo, no en su centro. */
function ellipseReach(rx: number, ry: number, ux: number, uy: number): number {
  const d = Math.hypot(uy * rx, ux * ry);
  return d === 0 ? 0 : (rx * ry) / d;
}

/** Reparte N tarjetas apiladas verticalmente que llenan la caja. */
function listLayout(n: number, vw: number, vh: number, pad: number): SmartArtGeometry {
  const innerW = Math.max(1, vw - 2 * pad);
  const avail = Math.max(1, vh - 2 * pad);
  const gap = clamp(avail * 0.05, 2, 14);
  // ponytail: con muchísimos items (~25+) las tarjetas se aplastan a 1px pero no
  // rompen; SmartArt de una diapositiva no lleva decenas de nodos.
  const cardH = Math.max(1, (avail - gap * (n - 1)) / n);
  const nodes: SmartArtNode[] = Array.from({ length: n }, (_, k) => ({
    i: k,
    x: pad,
    y: pad + k * (cardH + gap),
    w: innerW,
    h: cardH,
    shape: 'rect',
  }));
  return { nodes, connectors: [] };
}

/** Secuencia horizontal de cajas con una flecha entre cada par contiguo. */
function processLayout(n: number, vw: number, vh: number, pad: number): SmartArtGeometry {
  const innerH = Math.max(1, vh - 2 * pad);
  const avail = Math.max(1, vw - 2 * pad);
  const gap = clamp(avail * 0.06, 6, 48);
  const boxW = Math.max(1, (avail - gap * (n - 1)) / n);
  const cy = vh / 2;
  const nodes: SmartArtNode[] = Array.from({ length: n }, (_, k) => ({
    i: k,
    x: pad + k * (boxW + gap),
    y: pad,
    w: boxW,
    h: innerH,
    shape: 'rect',
  }));
  const connectors: SmartArtConnector[] = [];
  for (let k = 0; k < n - 1; k++) {
    const x1 = pad + k * (boxW + gap) + boxW;
    connectors.push({ x1, y1: cy, x2: x1 + gap, y2: cy, kind: 'arrow' });
  }
  return { nodes, connectors };
}

/** Nodos-elipse en un anillo con flechas en bucle (k → k+1, cerrando en 0). */
function cycleLayout(n: number, vw: number, vh: number, pad: number): SmartArtGeometry {
  const cx = vw / 2;
  const cy = vh / 2;
  const side = Math.min(vw, vh);
  const nodeW = clamp(side * (n <= 4 ? 0.32 : n <= 6 ? 0.26 : 0.2), 12, side * 0.5);
  const nodeH = nodeW * 0.66;
  if (n === 1) {
    return {
      nodes: [{ i: 0, x: cx - nodeW / 2, y: cy - nodeH / 2, w: nodeW, h: nodeH, shape: 'ellipse' }],
      connectors: [],
    };
  }
  const rx = Math.max(1, vw / 2 - pad - nodeW / 2);
  const ry = Math.max(1, vh / 2 - pad - nodeH / 2);
  const ncx: number[] = [];
  const ncy: number[] = [];
  const nodes: SmartArtNode[] = Array.from({ length: n }, (_, k) => {
    const a = -Math.PI / 2 + (k * 2 * Math.PI) / n; // arranca arriba, sentido horario
    const px = cx + rx * Math.cos(a);
    const py = cy + ry * Math.sin(a);
    ncx.push(px);
    ncy.push(py);
    return { i: k, x: px - nodeW / 2, y: py - nodeH / 2, w: nodeW, h: nodeH, shape: 'ellipse' };
  });
  const connectors: SmartArtConnector[] = [];
  const arw = nodeW / 2;
  const arh = nodeH / 2;
  // ponytail: con n===2 las dos flechas caen sobre la misma recta en sentidos
  // opuestos (se lee como bidireccional); no merece curvar el conector.
  for (let k = 0; k < n; k++) {
    const j = (k + 1) % n;
    const dx = ncx[j]! - ncx[k]!;
    const dy = ncy[j]! - ncy[k]!;
    const len = Math.hypot(dx, dy) || 1;
    const ux = dx / len;
    const uy = dy / len;
    const reach = ellipseReach(arw, arh, ux, uy);
    connectors.push({
      x1: ncx[k]! + ux * reach,
      y1: ncy[k]! + uy * reach,
      x2: ncx[j]! - ux * reach,
      y2: ncy[j]! - uy * reach,
      kind: 'arrow',
    });
  }
  return { nodes, connectors };
}

/* ---------- Jerarquía (organigrama) ---------- */

interface TreeNode {
  i: number;
  /** Profundidad efectiva (0 = raíz), derivada del árbol, no del `level` crudo. */
  depth: number;
  children: TreeNode[];
}

/** Convierte la lista plana con sangría en un bosque (varias raíces posibles):
 *  cada item cuelga del anterior con `level` estrictamente menor (regla de
 *  esquema). Saltos de sangría (0→2) se absorben: la profundidad efectiva es la
 *  del padre + 1, nunca deja huecos. */
function buildForest(items: SmartArtItem[]): TreeNode[] {
  const roots: TreeNode[] = [];
  const stack: { node: TreeNode; claimed: number }[] = [];
  items.forEach((it, i) => {
    const claimed = Math.max(0, Math.floor(it.level ?? 0));
    const node: TreeNode = { i, depth: 0, children: [] };
    while (stack.length > 0 && stack[stack.length - 1]!.claimed >= claimed) stack.pop();
    const parent = stack[stack.length - 1];
    if (parent) {
      node.depth = parent.node.depth + 1;
      parent.node.children.push(node);
    } else {
      roots.push(node);
    }
    stack.push({ node, claimed });
  });
  return roots;
}

/** Organigrama de arriba abajo: las hojas se reparten a lo ancho por igual y
 *  cada padre se centra sobre sus hijos; una fila por nivel de profundidad. */
function hierarchyLayout(
  items: SmartArtItem[],
  vw: number,
  vh: number,
  pad: number,
): SmartArtGeometry {
  const roots = buildForest(items);
  let leaves = 0;
  let maxDepth = 0;
  const measure = (nd: TreeNode): void => {
    if (nd.depth > maxDepth) maxDepth = nd.depth;
    if (nd.children.length === 0) leaves++;
    else nd.children.forEach(measure);
  };
  roots.forEach(measure);
  leaves = Math.max(1, leaves);
  const rows = maxDepth + 1;

  const innerW = Math.max(1, vw - 2 * pad);
  const innerH = Math.max(1, vh - 2 * pad);
  const slotW = innerW / leaves;
  const rowH = innerH / rows;
  const nodeW = Math.max(1, slotW * 0.86);
  const nodeH = Math.max(1, Math.min(rowH * 0.58, nodeW * 0.52));

  const centers = new Map<number, { cx: number; cy: number }>();
  const nodes: SmartArtNode[] = [];
  let leaf = 0;
  const place = (nd: TreeNode): void => {
    let cx: number;
    if (nd.children.length === 0) {
      cx = pad + (leaf + 0.5) * slotW;
      leaf++;
    } else {
      nd.children.forEach(place);
      const first = centers.get(nd.children[0]!.i)!;
      const last = centers.get(nd.children[nd.children.length - 1]!.i)!;
      cx = (first.cx + last.cx) / 2;
    }
    const cy = pad + (nd.depth + 0.5) * rowH;
    centers.set(nd.i, { cx, cy });
    nodes.push({
      i: nd.i,
      x: cx - nodeW / 2,
      y: cy - nodeH / 2,
      w: nodeW,
      h: nodeH,
      shape: 'rect',
    });
  };
  roots.forEach(place);

  const connectors: SmartArtConnector[] = [];
  const link = (nd: TreeNode): void => {
    const p = centers.get(nd.i)!;
    nd.children.forEach((ch) => {
      const c = centers.get(ch.i)!;
      connectors.push({
        x1: p.cx,
        y1: p.cy + nodeH / 2,
        x2: c.cx,
        y2: c.cy - nodeH / 2,
        kind: 'elbow',
      });
      link(ch);
    });
  };
  roots.forEach(link);

  // Orden estable por índice de item (nodos y texto se emparejan sin sorpresas).
  nodes.sort((a, b) => a.i - b.i);
  return { nodes, connectors };
}

/**
 * Auto-layout de un diagrama SmartArt: nodos + conectores en el espacio 0..vw ×
 * 0..vh de la caja. `items` vacío (o caja degenerada) → geometría vacía; el
 * llamante pinta entonces un aviso. Pura y exportada para el self-test (bun test).
 */
export function smartArtLayout(
  layout: SmartArtLayout,
  items: SmartArtItem[],
  vw: number,
  vh: number,
): SmartArtGeometry {
  const n = items.length;
  if (n === 0 || vw <= 0 || vh <= 0) return { nodes: [], connectors: [] };
  const pad = clamp(Math.min(vw, vh) * 0.06, 4, 22);
  switch (layout) {
    case 'process':
      return processLayout(n, vw, vh, pad);
    case 'cycle':
      return cycleLayout(n, vw, vh, pad);
    case 'hierarchy':
      return hierarchyLayout(items, vw, vh, pad);
    case 'list':
    default:
      return listLayout(n, vw, vh, pad);
  }
}
