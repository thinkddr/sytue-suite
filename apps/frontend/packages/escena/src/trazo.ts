/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mano alzada: de los puntos crudos del puntero (ratón, dedo o lápiz) a un trazo
 * limpio. Un puntero de alta frecuencia da cientos de puntos por segundo con ruido
 * de ±1 px: guardarlos tal cual infla el documento y dibuja temblores.
 *
 *  1. `limpiar` quita los puntos más cercanos que `minimo` al anterior (puntero
 *     parado) y los que no se apartan más de `tolerancia` de la recta entre sus
 *     vecinos (Ramer–Douglas–Peucker con pila explícita: un trazo largo no desborda
 *     la pila).
 *  2. El pintado (`trazoPath` en svg.ts) une lo que queda con cuadráticas por los
 *     puntos medios: sale suave sin tocar los datos.
 *
 * Puro: números planos [x0, y0, x1, y1, …] dentro y fuera.
 */

function distSeg(px: number, py: number, ax: number, ay: number, bx: number, by: number): number {
  const dx = bx - ax;
  const dy = by - ay;
  const l2 = dx * dx + dy * dy;
  const t = l2 === 0 ? 0 : Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / l2));
  return Math.hypot(px - (ax + t * dx), py - (ay + t * dy));
}

const r1 = (n: number): number => Math.round(n * 10) / 10;

export interface OpcionesTrazo {
  /** Un punto a menos de esta distancia del anterior se descarta. Defecto 0,5. */
  minimo?: number;
  /** Desviación máxima respecto al trazo original. Defecto 0,8 unidades de escena. */
  tolerancia?: number;
}

/** Trazo limpio y simplificado: conserva el primer y el último punto y el orden; un toque
 *  (un punto) se conserva y se pinta como un punto redondo. */
export function limpiar(puntos: readonly number[], opc: OpcionesTrazo = {}): number[] {
  const minimo = opc.minimo ?? 0.5;
  const tol = opc.tolerancia ?? 0.8;
  const xs: number[] = [];
  const ys: number[] = [];
  let ultimoX = 0;
  let ultimoY = 0;
  let descartado = false;
  for (let i = 0; i + 1 < puntos.length; i += 2) {
    const x = puntos[i]!;
    const y = puntos[i + 1]!;
    if (!Number.isFinite(x) || !Number.isFinite(y)) continue;
    const n = xs.length;
    if (n > 0 && Math.hypot(x - xs[n - 1]!, y - ys[n - 1]!) < minimo) {
      ultimoX = x;
      ultimoY = y;
      descartado = true;
      continue;
    }
    descartado = false;
    xs.push(x);
    ys.push(y);
  }
  // El trazo acaba donde el usuario soltó, aunque ese punto fuera casi el anterior.
  if (descartado && xs.length > 1) {
    xs[xs.length - 1] = ultimoX;
    ys[ys.length - 1] = ultimoY;
  }
  const n = xs.length;
  if (n <= 2) return xs.flatMap((x, i) => [r1(x), r1(ys[i]!)]);
  const guardar = new Uint8Array(n);
  guardar[0] = 1;
  guardar[n - 1] = 1;
  const pila: [number, number][] = [[0, n - 1]];
  while (pila.length > 0) {
    const [a, b] = pila.pop()!;
    let maxD = 0;
    let idx = -1;
    for (let i = a + 1; i < b; i++) {
      const d = distSeg(xs[i]!, ys[i]!, xs[a]!, ys[a]!, xs[b]!, ys[b]!);
      if (d > maxD) {
        maxD = d;
        idx = i;
      }
    }
    if (idx !== -1 && maxD > tol) {
      guardar[idx] = 1;
      pila.push([a, idx], [idx, b]);
    }
  }
  const out: number[] = [];
  for (let i = 0; i < n; i++) if (guardar[i]) out.push(r1(xs[i]!), r1(ys[i]!));
  return out;
}
