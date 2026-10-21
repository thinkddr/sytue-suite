/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones lógicas (IF, IFS, AND, OR, NOT, XOR, TRUE, FALSE, IFERROR, IFNA,
 * SWITCH). Semántica OpenFormula / ECMA-376, código original (clean-room).
 *
 * EVALUACIÓN PEREZOSA (crítico): cada `Arg` se evalúa bajo demanda, así que una
 * función solo llama `value()`/`matrix()` de los argumentos que necesita:
 *   - IF/IFS/SWITCH evalúan la condición y SOLO la rama/resultado elegido.
 *   - IFERROR/IFNA evalúan el argumento de reserva solo si el primero falla.
 * AND/OR/XOR, en cambio, son EAGER como en una hoja real: evalúan TODOS sus
 * argumentos y propagan el primer #error (p. ej. =AND(FALSO;1/0) → #DIV/0!); no
 * hacen cortocircuito lógico.
 */
import type { Arg, FnTable } from '../registry.js';
import { coerceBoolean, compareValues, ERR, isError, type FormulaError } from '../values.js';

/**
 * Recoge los booleanos de los argumentos de AND/OR/XOR. Escalar directo →
 * coacción a booleano (texto TRUE/FALSE admitido). Dentro de una referencia o
 * rango se recorren las celdas ignorando texto y blancos (regla de hoja) y los
 * números cuentan como (≠0). Propaga el primer #error; si no hay ningún valor
 * lógico devuelve la lista vacía (los llamadores lo tratan como #VALUE!).
 */
function gatherBooleans(args: Arg[]): boolean[] | FormulaError {
  const out: boolean[] = [];
  for (const a of args) {
    if (a.ref !== undefined) {
      for (const row of a.matrix()) {
        for (const v of row) {
          if (isError(v)) return v;
          if (typeof v === 'boolean') out.push(v);
          else if (typeof v === 'number') out.push(v !== 0);
          // texto y blancos dentro de una referencia: ignorados
        }
      }
    } else {
      const b = coerceBoolean(a.value());
      if (isError(b)) return b;
      out.push(b);
    }
  }
  return out;
}

export const funcs: FnTable = {
  // Perezoso: evalúa la condición y SOLO la rama tomada; si falta el «si no»,
  // una condición falsa devuelve FALSE (como Excel).
  IF(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const cond = coerceBoolean(args[0]!.value());
    if (isError(cond)) return cond;
    if (cond) return args[1]!.value();
    return args.length === 3 ? args[2]!.value() : false;
  },

  // Perezoso: primera condición verdadera gana; #N/A si ninguna se cumple.
  IFS(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    for (let i = 0; i + 1 < args.length; i += 2) {
      const cond = coerceBoolean(args[i]!.value());
      if (isError(cond)) return cond;
      if (cond) return args[i + 1]!.value();
    }
    return ERR.NA;
  },

  // Eager: todos verdaderos → TRUE. Sin valores lógicos → #VALUE!.
  AND(_ctx, args) {
    const bs = gatherBooleans(args);
    if (!Array.isArray(bs)) return bs;
    if (bs.length === 0) return ERR.VALUE;
    return bs.every((b) => b);
  },

  // Eager: algún verdadero → TRUE. Sin valores lógicos → #VALUE!.
  OR(_ctx, args) {
    const bs = gatherBooleans(args);
    if (!Array.isArray(bs)) return bs;
    if (bs.length === 0) return ERR.VALUE;
    return bs.some((b) => b);
  },

  // Eager: verdadero si un número IMPAR de argumentos es verdadero.
  XOR(_ctx, args) {
    const bs = gatherBooleans(args);
    if (!Array.isArray(bs)) return bs;
    if (bs.length === 0) return ERR.VALUE;
    let count = 0;
    for (const b of bs) if (b) count++;
    return count % 2 === 1;
  },

  NOT(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const b = coerceBoolean(args[0]!.value());
    return isError(b) ? b : !b;
  },

  // TRUE()/FALSE() constantes (con paréntesis el parser las trata como llamada).
  TRUE() {
    return true;
  },
  FALSE() {
    return false;
  },

  // Perezoso: captura CUALQUIER #error del primer argumento y devuelve el de
  // reserva; si no hay error, devuelve el valor sin tocar el de reserva.
  IFERROR(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const v = args[0]!.value();
    return isError(v) ? args[1]!.value() : v;
  },

  // Perezoso: como IFERROR pero SOLO captura #N/A; otros errores se propagan.
  IFNA(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const v = args[0]!.value();
    if (isError(v) && v.code === '#N/A') return args[1]!.value();
    return v;
  },

  // Perezoso: SWITCH(expr, valor1, resultado1, …, [predeterminado]). Compara la
  // expresión con cada valor (igualdad de hoja, texto insensible a mayúsculas) y
  // devuelve el primer resultado que empareje; un argumento suelto final es el
  // predeterminado. Sin emparejar y sin predeterminado → #N/A.
  SWITCH(_ctx, args) {
    if (args.length < 1) return ERR.VALUE;
    const subject = args[0]!.value();
    if (isError(subject)) return subject;
    for (let i = 1; i + 1 < args.length; i += 2) {
      const cmp = compareValues(subject, args[i]!.value());
      if (isError(cmp)) return cmp;
      if (cmp === 0) return args[i + 1]!.value();
    }
    if ((args.length - 1) % 2 === 1) return args[args.length - 1]!.value();
    return ERR.NA;
  },
};
