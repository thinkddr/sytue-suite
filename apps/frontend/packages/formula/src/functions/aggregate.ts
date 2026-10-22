/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SUBTOTAL y AGGREGATE: las agregaciones que saben de ESQUEMA. Hacen lo mismo que
 * SUM, AVERAGE, MAX… con tres diferencias que son toda su razón de ser:
 *
 *  - Se saltan las celdas que ya son un SUBTOTAL o un AGGREGATE. Por eso el total
 *    general de un listado con subtotales es `SUBTOTAL(9; B2:B40)` sobre TODO el
 *    rango: los subtotales de en medio no se cuentan dos veces.
 *  - Se saltan las filas que esconde el filtro (siempre en SUBTOTAL; en AGGREGATE,
 *    según la opción).
 *  - Las 101-111 de SUBTOTAL y las opciones impares de AGGREGATE se saltan también
 *    las filas ocultas a mano, que aquí son las de un grupo contraído.
 *
 * No reimplementan ninguna agregación: construyen el rango sin lo que hay que
 * saltar (las celdas saltadas pasan a estar VACÍAS, que es lo que toda agregación ya
 * ignora) y llaman a la función de siempre. Así SUBTOTAL(1; …) y AVERAGE no pueden
 * divergir nunca en un caso raro.
 *
 * Qué es anidado y qué está oculto lo dice el motor (`FnCtx.subtotalCell` y
 * `FnCtx.rowHidden`); el editor le cuenta las filas ocultas con `setHiddenRows`.
 * Vectores compartidos con Go: apps/nucleo/formula/testdata/subtotal.json.
 */
import type { Arg, FnCtx, FnImpl, FnTable } from '../registry.js';
import { coerceNumber, ERR, isError, type CellValue, type FormulaError } from '../values.js';
import { funcs as math } from './math.js';
import { funcs as stats } from './stats.js';

/** Las once de SUBTOTAL, por su número (1-11; 101-111 son las mismas). */
const DE_SUBTOTAL = [
  'AVERAGE',
  'COUNT',
  'COUNTA',
  'MAX',
  'MIN',
  'PRODUCT',
  'STDEV',
  'STDEVP',
  'SUM',
  'VAR',
  'VARP',
] as const;

/** Las diecinueve de AGGREGATE. De la 14 a la 19 llevan un segundo argumento `k`. */
const DE_AGGREGATE = [
  'AVERAGE',
  'COUNT',
  'COUNTA',
  'MAX',
  'MIN',
  'PRODUCT',
  'STDEV.S',
  'STDEV.P',
  'SUM',
  'VAR.S',
  'VAR.P',
  'MEDIAN',
  'MODE.SNGL',
  'LARGE',
  'SMALL',
  'PERCENTILE.INC',
  'QUARTILE.INC',
  'PERCENTILE.EXC',
  'QUARTILE.EXC',
] as const;

const TODAS: FnTable = { ...math, ...stats };

/** Qué se salta al leer un rango. `ocultas`: 0 nada, 1 las filtradas, 2 también las de a mano. */
interface Salto {
  anidadas: boolean;
  ocultas: 0 | 1 | 2;
  errores: boolean;
}

/** El argumento con lo saltado convertido en celdas vacías. Si no es una referencia, igual. */
function sinSaltadas(ctx: FnCtx, a: Arg, s: Salto): Arg {
  const rr = a.ref;
  const m = a.matrix();
  const fuera = (v: CellValue, c: number, r: number): boolean => {
    if (s.errores && isError(v)) return true;
    if (!rr) return false;
    if (s.anidadas && ctx.subtotalCell?.(rr.sheet, c, r)) return true;
    // rowHidden: 2 filtrada, 1 a mano. `ocultas`: 1 salta las filtradas; 2, las dos.
    const h = ctx.rowHidden?.(rr.sheet, r) ?? 0;
    return (h === 2 && s.ocultas >= 1) || (h === 1 && s.ocultas === 2);
  };
  const out = m.map((fila, i) =>
    fila.map((v, j) => (fuera(v, (rr?.c1 ?? 0) + j, (rr?.r1 ?? 0) + i) ? '' : v)),
  );
  return { value: () => out[0]?.[0] ?? '', matrix: () => out, ref: rr };
}

/** El número de función: entero truncado, o el error que toque. */
function numero(a: Arg | undefined): number | FormulaError {
  if (!a) return ERR.VALUE;
  const n = coerceNumber(a.value());
  return isError(n) ? n : Math.trunc(n);
}

export const funcs: FnTable = {
  /**
   * SUBTOTAL(función; ref1; [ref2]; …). 1-11 saltan lo filtrado y los resúmenes
   * anidados; 101-111, además, lo oculto a mano. Solo admite referencias: un rango
   * calculado no tiene filas que puedan estar ocultas, y Excel lo rechaza igual.
   */
  SUBTOTAL(ctx, args) {
    const n = numero(args[0]);
    if (isError(n)) return n;
    const base = n > 100 ? n - 100 : n;
    const nombre = DE_SUBTOTAL[base - 1];
    const refs = args.slice(1);
    if (!nombre || refs.length === 0 || refs.some((a) => a.ref === undefined)) return ERR.VALUE;
    const salto: Salto = { anidadas: true, ocultas: n > 100 ? 2 : 1, errores: false };
    return (TODAS[nombre] as FnImpl)(
      ctx,
      refs.map((a) => sinSaltadas(ctx, a, salto)),
    );
  },

  /**
   * AGGREGATE(función; opciones; ref1; …) y AGGREGATE(función; opciones; matriz; k).
   * Opciones 0-7: con 0-3 se saltan los resúmenes anidados; con las impares, las filas
   * ocultas (filtradas o a mano); con 2, 3, 6 y 7, los errores. Omitida vale 0.
   */
  AGGREGATE(ctx, args) {
    const n = numero(args[0]);
    if (isError(n)) return n;
    const opcion = args[1] === undefined ? 0 : numero(args[1]);
    if (isError(opcion)) return opcion;
    const nombre = DE_AGGREGATE[n - 1];
    if (!nombre || opcion < 0 || opcion > 7) return ERR.VALUE;
    const salto: Salto = {
      anidadas: opcion < 4,
      ocultas: opcion % 2 === 1 ? 2 : 0,
      errores: opcion === 2 || opcion === 3 || opcion === 6 || opcion === 7,
    };
    const impl = TODAS[nombre] as FnImpl;
    if (n >= 14) {
      // Forma de matriz: una matriz (o rango) y la k. Sin k no hay nada que calcular.
      const [datos, k] = [args[2], args[3]];
      if (!datos || !k || args.length > 4) return ERR.VALUE;
      return impl(ctx, [sinSaltadas(ctx, datos, salto), k]);
    }
    const refs = args.slice(2);
    if (refs.length === 0 || refs.some((a) => a.ref === undefined)) return ERR.VALUE;
    return impl(
      ctx,
      refs.map((a) => sinSaltadas(ctx, a, salto)),
    );
  },
};
