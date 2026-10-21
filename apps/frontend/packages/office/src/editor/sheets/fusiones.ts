/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CELDAS COMBINADAS de Calcus (`SheetData.merges`): rangos A1 de más de una celda que no
 * se solapan. La celda de arriba a la izquierda es el ANCLA (la que se pinta, ocupando todo
 * el rango); las demás quedan cubiertas. Puro, sin DOM: lo usan SheetCanvas y los tests.
 * Self-test: packages/office/test/sheets-fusiones.test.ts (bun test).
 */
import { parseRangeRef } from './format';

export interface Fusion {
  c1: number;
  r1: number;
  c2: number;
  r2: number;
}

/** Tope de combinadas por hoja (lo que importa un .xlsx o un .ods no crece sin medida). */
export const MAX_FUSIONES = 5000;

/** Las combinadas válidas de la hoja: más de una celda, sin solaparse (la primera gana). */
export function parseFusiones(merges: readonly string[] | undefined): Fusion[] {
  const out: Fusion[] = [];
  for (const m of merges ?? []) {
    if (out.length >= MAX_FUSIONES) break;
    const r = parseRangeRef(m);
    if (!r || (r.c1 === r.c2 && r.r1 === r.r2)) continue;
    if (out.some((o) => r.c1 <= o.c2 && r.c2 >= o.c1 && r.r1 <= o.r2 && r.r2 >= o.r1)) continue;
    out.push({ c1: r.c1, r1: r.r1, c2: r.c2, r2: r.r2 });
  }
  return out;
}

/** La combinada que cubre la celda (ancla incluida), o null. */
export function fusionEn(fs: readonly Fusion[], c: number, r: number): Fusion | null {
  for (const f of fs) if (c >= f.c1 && c <= f.c2 && r >= f.r1 && r <= f.r2) return f;
  return null;
}

/** Las que tocan el rectángulo [c1..c2] × [r1..r2] (lo que se pinta), hasta `tope`. */
export function fusionesEnVentana(
  fs: readonly Fusion[],
  c1: number,
  r1: number,
  c2: number,
  r2: number,
  tope = 400,
): Fusion[] {
  const out: Fusion[] = [];
  for (const f of fs) {
    if (f.c1 <= c2 && f.c2 >= c1 && f.r1 <= r2 && f.r2 >= r1) {
      out.push(f);
      if (out.length >= tope) break;
    }
  }
  return out;
}

/**
 * A dónde va una celda tras un movimiento de teclado de (dc, dr): si sale de una combinada
 * se salta entera (Excel), y si cae en una, se queda en su ancla.
 */
export function moverPorFusiones(
  fs: readonly Fusion[],
  base: { c: number; r: number },
  dc: number,
  dr: number,
  maxC: number,
  maxR: number,
): { c: number; r: number } {
  const desde = fusionEn(fs, base.c, base.r);
  let c = base.c;
  let r = base.r;
  if (desde) {
    if (dc > 0) c = desde.c2;
    else if (dc < 0) c = desde.c1;
    if (dr > 0) r = desde.r2;
    else if (dr < 0) r = desde.r1;
  }
  c = Math.min(maxC, Math.max(0, c + dc));
  r = Math.min(maxR, Math.max(0, r + dr));
  const hasta = fusionEn(fs, c, r);
  return hasta ? { c: hasta.c1, r: hasta.r1 } : { c, r };
}
