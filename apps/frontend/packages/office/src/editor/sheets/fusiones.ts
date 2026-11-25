/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CELDAS COMBINADAS de Calcus (`SheetData.merges`): rangos A1 de más de una celda que no
 * se solapan. La celda de arriba a la izquierda es el ANCLA (la que se pinta, ocupando todo
 * el rango); las demás quedan cubiertas. Puro, sin DOM: lo usan SheetCanvas y los tests.
 * Self-test: packages/office/test/sheets-fusiones.test.ts (bun test).
 */
import { numToCol } from '@sytue/formula/motor';
import { parseCell, parseRangeRef, renderCellHtml, stripHtml } from './format';

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

/** «B2:D3» de una combinada (lo que se guarda en `SheetData.merges`). */
export function fusionARango(f: Fusion): string {
  return `${numToCol(f.c1)}${f.r1 + 1}:${numToCol(f.c2)}${f.r2 + 1}`;
}

const tocan = (a: Fusion, b: Fusion): boolean =>
  a.c1 <= b.c2 && a.c2 >= b.c1 && a.r1 <= b.r2 && a.r2 >= b.r1;

/**
 * COMBINAR el rectángulo: devuelve la lista nueva de `merges` y la combinada que queda, o
 * null si no hay nada que combinar (una sola celda). Como Excel, si la selección pisa a
 * medias otra combinada se agranda hasta englobarla, y las que quedan dentro se sustituyen
 * por la nueva: dos combinadas solapadas no pueden convivir (parseFusiones descartaría la
 * segunda y la hoja pintaría otra cosa de lo que se pidió).
 */
export function combinar(
  merges: readonly string[] | undefined,
  rect: Fusion,
): { merges: string[]; fusion: Fusion } | null {
  const existentes = parseFusiones(merges);
  const f: Fusion = { c1: rect.c1, r1: rect.r1, c2: rect.c2, r2: rect.r2 };
  // Agrandar puede alcanzar otra combinada: se repite hasta que no cambie.
  for (let cambia = true; cambia;) {
    cambia = false;
    for (const o of existentes) {
      if (!tocan(f, o)) continue;
      if (o.c1 < f.c1 || o.r1 < f.r1 || o.c2 > f.c2 || o.r2 > f.r2) {
        f.c1 = Math.min(f.c1, o.c1);
        f.r1 = Math.min(f.r1, o.r1);
        f.c2 = Math.max(f.c2, o.c2);
        f.r2 = Math.max(f.r2, o.r2);
        cambia = true;
      }
    }
  }
  if (f.c1 === f.c2 && f.r1 === f.r2) return null;
  const quedan = (merges ?? []).filter((m) => {
    const r = parseRangeRef(m);
    return r === null || !tocan(f, r);
  });
  // Con la hoja en el tope, parseFusiones descartaría la nueva: el lienzo no la pintaría
  // y combinarHoja ya habría vaciado las celdas cubiertas. Mejor no combinar.
  if (parseFusiones(quedan).length >= MAX_FUSIONES) return null;
  return { merges: [...quedan, fusionARango(f)], fusion: f };
}

/**
 * SEPARAR: quita las combinadas que toca el rectángulo (basta con tener seleccionada su
 * ancla). Null si no había ninguna, para no anotar un paso de deshacer que no hace nada.
 */
export function separar(merges: readonly string[] | undefined, rect: Fusion): string[] | null {
  const quedan = (merges ?? []).filter((m) => {
    const r = parseRangeRef(m);
    return r === null || !tocan(rect, r);
  });
  return quedan.length === (merges ?? []).length ? null : quedan;
}

/**
 * Las celdas guardadas que quedan CUBIERTAS por la combinada (todas menos el ancla). Se
 * recorre el mapa disperso y no el rango: combinar una columna entera no puede iterar un
 * millón de filas vacías.
 */
export function celdasCubiertas(cells: Readonly<Record<string, string>>, f: Fusion): string[] {
  const out: string[] = [];
  for (const ref of Object.keys(cells)) {
    const p = parseRangeRef(ref);
    if (!p || p.c1 !== p.c2 || p.r1 !== p.r2) continue;
    if (p.c1 < f.c1 || p.c1 > f.c2 || p.r1 < f.r1 || p.r1 > f.r2) continue;
    if (p.c1 === f.c1 && p.r1 === f.r1) continue;
    out.push(ref);
  }
  return out;
}

/** ¿Combinar el rectángulo vaciaría alguna celda con algo escrito? (Entonces se avisa.) */
export function combinarPierdeDatos(cells: Readonly<Record<string, string>>, f: Fusion): boolean {
  return celdasCubiertas(cells, f).some((ref) => stripHtml(cells[ref] ?? '').trim() !== '');
}

/**
 * Lo que hace «Combinar» sobre una hoja: la combinada nueva en `merges` y las celdas
 * cubiertas vaciadas —solo se conserva el ancla, como en Excel y Sheets; el formato de las
 * cubiertas se queda, que es lo que reaparece al separar—. Con `centrar`, el texto del ancla
 * se centra («Combinar y centrar»). Null si no hay nada que combinar.
 */
export function combinarHoja(
  hoja: { cells: Readonly<Record<string, string>>; merges?: readonly string[] | undefined },
  rect: Fusion,
  centrar: boolean,
): { cells: Record<string, string>; merges: string[]; fusion: Fusion } | null {
  const plan = combinar(hoja.merges, rect);
  if (!plan) return null;
  const f = plan.fusion;
  const cells = { ...hoja.cells };
  for (const ref of celdasCubiertas(cells, f)) delete cells[ref];
  const ancla = `${numToCol(f.c1)}${f.r1 + 1}`;
  const html = cells[ancla];
  if (centrar && html !== undefined) {
    const p = parseCell(html);
    if (p.inner !== '') {
      p.align = 'center';
      cells[ancla] = renderCellHtml(p);
    }
  }
  return { cells, merges: plan.merges, fusion: f };
}
