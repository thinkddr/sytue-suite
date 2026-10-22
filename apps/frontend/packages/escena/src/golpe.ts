/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Hit-testing de la escena: qué nodo hay bajo un punto (clic, toque o lápiz) y
 * qué nodos toca o contiene un rectángulo (selección por marco). Puro: las
 * capas ocultas o bloqueadas no se pueden tocar; de arriba abajo en Z.
 */
import { polygonPoints } from './formas.js';
import { aplicar, invertir } from './matriz.js';
import { cajaMundo, matrizDeCaja, rutaDe } from './escena.js';
import type { NodoDeCaja } from './escena.js';
import type { Rect } from './organizar.js';
import { GROSOR_DEFECTO } from './tipos.js';
import type { Escena, Nodo, Punto } from './tipos.js';

/** Nodo de primer nivel alcanzado (un clic en un hijo de un grupo selecciona el grupo). */
export interface Golpe {
  id: string;
  capaId: string;
}

/** Distancia de `p` al segmento a–b. */
export function distSegmento(p: Punto, a: Punto, b: Punto): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const l2 = dx * dx + dy * dy;
  const t = l2 === 0 ? 0 : Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / l2));
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy));
}

const distPolilinea = (p: Punto, pts: readonly Punto[]): number => {
  if (pts.length === 0) return Infinity;
  if (pts.length === 1) return Math.hypot(p.x - pts[0]!.x, p.y - pts[0]!.y);
  let d = Infinity;
  for (let i = 0; i + 1 < pts.length; i++) d = Math.min(d, distSegmento(p, pts[i]!, pts[i + 1]!));
  return d;
};

/** Punto dentro de un polígono (par-impar). */
function dentroDePoligono(p: Punto, pts: readonly (readonly [number, number])[]): boolean {
  let dentro = false;
  for (let i = 0, j = pts.length - 1; i < pts.length; j = i++) {
    const [xi, yi] = pts[i]!;
    const [xj, yj] = pts[j]!;
    if (yi > p.y !== yj > p.y && p.x < ((xj - xi) * (p.y - yi)) / (yj - yi) + xi) dentro = !dentro;
  }
  return dentro;
}

/** Puntos de un trazado SVG de conector (M/L/C absolutos) como polilínea; las
 *  Bézier cúbicas se muestrean. */
export function puntosDeRuta(d: string): Punto[] {
  const t = d.match(/[MLC]|-?\d+(?:\.\d+)?/g) ?? [];
  const out: Punto[] = [];
  let i = 0;
  let cmd = '';
  const n = (): number => Number(t[i++]);
  while (i < t.length) {
    if (/[MLC]/.test(t[i]!)) cmd = t[i++]!;
    if (cmd === 'C') {
      const p0 = out[out.length - 1] ?? { x: 0, y: 0 };
      const c1 = { x: n(), y: n() };
      const c2 = { x: n(), y: n() };
      const p3 = { x: n(), y: n() };
      for (let k = 1; k <= 16; k++) {
        const s = k / 16;
        const u = 1 - s;
        const f = (a: number, b: number, c: number, e: number): number =>
          u * u * u * a + 3 * u * u * s * b + 3 * u * s * s * c + s * s * s * e;
        out.push({ x: f(p0.x, c1.x, c2.x, p3.x), y: f(p0.y, c1.y, c2.y, p3.y) });
      }
    } else {
      out.push({ x: n(), y: n() });
    }
  }
  return out;
}

function tocaCaja(n: NodoDeCaja, q: Punto, tol: number, escena: Escena): boolean {
  const { w, h } = n;
  if (n.tipo === 'grupo') {
    for (let i = n.hijos.length - 1; i >= 0; i--)
      if (toca(escena, n.hijos[i]!, q, tol)) return true;
    return false;
  }
  if (n.tipo === 'forma') {
    if (n.forma === 'ellipse') {
      const rx = w / 2 + tol;
      const ry = h / 2 + tol;
      return ((q.x - w / 2) / rx) ** 2 + ((q.y - h / 2) / ry) ** 2 <= 1;
    }
    const poli = polygonPoints(n.forma, w, h);
    if (poli.length > 0) {
      if (dentroDePoligono(q, poli)) return true;
      const cerrado = [...poli, poli[0]!].map(([x, y]) => ({ x, y }));
      return distPolilinea(q, cerrado) <= tol;
    }
  }
  return q.x >= -tol && q.x <= w + tol && q.y >= -tol && q.y <= h + tol;
}

/** ¿Toca el punto `p` (en el sistema del padre del nodo) al nodo `n`? */
function toca(escena: Escena, n: Nodo, p: Punto, tol: number): boolean {
  const gr = (n.estilo?.grosor ?? GROSOR_DEFECTO) / 2 + tol;
  switch (n.tipo) {
    case 'linea':
      return distSegmento(p, { x: n.x1, y: n.y1 }, { x: n.x2, y: n.y2 }) <= gr;
    case 'trazo': {
      const pts: Punto[] = [];
      for (let i = 0; i + 1 < n.puntos.length; i += 2)
        pts.push({ x: n.puntos[i]!, y: n.puntos[i + 1]! });
      return distPolilinea(p, pts) <= gr;
    }
    case 'conector':
      return distPolilinea(p, puntosDeRuta(rutaDe(escena, n).d)) <= gr;
    default: {
      const inv = invertir(matrizDeCaja(n));
      return inv ? tocaCaja(n, aplicar(inv, p), tol, escena) : false;
    }
  }
}

/** Nodo de primer nivel más alto bajo (x, y), con `tol` unidades de margen. */
export function golpear(escena: Escena, x: number, y: number, tol = 3): Golpe | undefined {
  const p = { x, y };
  for (let c = escena.capas.length - 1; c >= 0; c--) {
    const capa = escena.capas[c]!;
    if (!capa.visible || capa.bloqueada) continue;
    for (let i = capa.nodos.length - 1; i >= 0; i--) {
      const n = capa.nodos[i]!;
      if (toca(escena, n, p, tol)) return { id: n.id, capaId: capa.id };
    }
  }
  return undefined;
}

/** Ids de los nodos de primer nivel que el marco `r` contiene por completo
 *  (`dentro`) o toca (`toca`), de las capas visibles y desbloqueadas. */
export function nodosEnRect(escena: Escena, r: Rect, modo: 'dentro' | 'toca' = 'toca'): string[] {
  const out: string[] = [];
  for (const capa of escena.capas) {
    if (!capa.visible || capa.bloqueada) continue;
    for (const n of capa.nodos) {
      const b = cajaMundo(escena, n.id);
      if (!b) continue;
      const dentro = b.x >= r.x && b.y >= r.y && b.x + b.w <= r.x + r.w && b.y + b.h <= r.y + r.h;
      const cruza = b.x <= r.x + r.w && b.x + b.w >= r.x && b.y <= r.y + r.h && b.y + b.h >= r.y;
      if (modo === 'dentro' ? dentro : cruza) out.push(n.id);
    }
  }
  return out;
}
