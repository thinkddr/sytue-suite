/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones de información (ISBLANK, ISNUMBER, ISTEXT, ISNONTEXT, ISLOGICAL,
 * ISERROR, ISERR, ISNA, ISREF, ISEVEN, ISODD, NA, N, TYPE, ERROR.TYPE).
 * Inspeccionan el TIPO/forma de un valor: las IS* NO coaccionan ni propagan
 * #errores, los EXAMINAN (por eso ISERROR(1/0)=TRUE en vez de propagar). ISREF
 * mira la FORMA del argumento (¿es referencia?), sin evaluar su valor. ISEVEN/
 * ISODD sí coaccionan a número y propagan. Semántica OpenFormula / ECMA-376,
 * código original (clean-room).
 *
 * Modelo (ver values.ts, nota 4): una celda en blanco vale '' y no se distingue
 * de la cadena vacía. Por eso ISBLANK('')=TRUE, ISTEXT('')=FALSE, ISNONTEXT('')=
 * TRUE y TYPE('')=1, replicando cómo trata Excel una celda vacía.
 *
 * ISFORMULA usa `ctx.isFormula(ref)` (el motor sabe si la celda superior-
 * izquierda de la referencia es una fórmula «=»); inspecciona la FORMA con
 * arg.ref, sin evaluar el valor, igual que ISREF.
 */
import type { FnImpl, FnTable } from '../registry.js';
import { ERR, isError, coerceNumber, coerceText, type CellValue } from '../values.js';
import { numToCol } from '../parser.js';

/**
 * Envuelve un predicado unario de INSPECCIÓN: valida que haya exactamente un
 * argumento y le pasa su valor SIN coaccionar. A diferencia de casi todas las
 * funciones, las IS* no propagan #errores: los examinan (p. ej. ISERROR/ISNA).
 */
function is1(pred: (v: CellValue) => boolean): FnImpl {
  return (_ctx, args) => (args.length === 1 ? pred(args[0]!.value()) : ERR.VALUE);
}

/** ERROR.TYPE: código de error → número de clasificación (Excel/ECMA-376). */
const ERRTYPE: Record<string, number> = {
  '#NULL!': 1,
  '#DIV/0!': 2,
  '#VALUE!': 3,
  '#REF!': 4,
  '#NAME?': 5,
  '#NUM!': 6,
  '#N/A': 7,
  '#SPILL!': 9,
  '#CALC!': 14,
  '#CIRC!': 4, // ponytail: #CIRC! ≈ #REF! circular (ver values.ts)
};

export const funcs: FnTable = {
  // Blanco: el valor es '' (celda vacía en el modelo de texto).
  ISBLANK: is1((v) => v === ''),
  ISNUMBER: is1((v) => typeof v === 'number'),
  // Texto NO vacío: '' se trata como blanco, no como texto (como una celda vacía).
  ISTEXT: is1((v) => typeof v === 'string' && v !== ''),
  ISNONTEXT: is1((v) => !(typeof v === 'string' && v !== '')),
  ISLOGICAL: is1((v) => typeof v === 'boolean'),
  ISERROR: is1((v) => isError(v)),
  ISERR: is1((v) => isError(v) && v.code !== '#N/A'),
  ISNA: is1((v) => isError(v) && v.code === '#N/A'),

  // ISREF: ¿el argumento es una referencia/rango LITERAL (A1, A1:B2, Hoja!A1)?
  // Inspecciona la FORMA con arg.ref, sin evaluar el valor (así ISREF(1/0) da
  // FALSE, no #DIV/0!). NO usa is1 porque is1 llamaría value().
  // ponytail: ISREF(INDIRECT("A1")) da FALSE (Excel: TRUE). El grafo de refs es
  // estático y una llamada no expone ref; corregir requeriría refs dinámicas.
  ISREF: (_ctx, args) => (args.length === 1 ? args[0]!.ref !== undefined : ERR.VALUE),

  // ISFORMULA(ref): ¿la celda (superior-izquierda del rango) es una fórmula «=»?
  // Requiere una referencia literal; si no lo es → #VALUE! (como Excel).
  ISFORMULA(ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const ref = args[0]!.ref;
    if (!ref) return ERR.VALUE;
    return ctx.isFormula(ref);
  },

  // ISEVEN/ISODD: SÍ coaccionan a número (blanco → 0) y propagan #errores. El
  // número se trunca hacia cero antes de comprobar la paridad (regla de Excel).
  ISEVEN(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    return isError(n) ? n : Math.trunc(n) % 2 === 0;
  },
  ISODD(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    return isError(n) ? n : Math.trunc(n) % 2 !== 0;
  },

  // Constante de error #N/A (marca «sin dato»). Sin argumentos.
  NA() {
    return ERR.NA;
  },

  // N: número→él mismo, booleano→1/0, error→se propaga; CUALQUIER texto → 0
  // (a diferencia de la coacción normal, N NO parsea «7» como número).
  N(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const v = args[0]!.value();
    if (isError(v)) return v;
    if (typeof v === 'number') return v;
    if (typeof v === 'boolean') return v ? 1 : 0;
    return 0;
  },

  // TYPE: 1 número, 2 texto, 4 lógico, 16 error, 64 matriz (rango multi-celda).
  // El blanco '' cuenta como número (1), como Excel con una celda vacía.
  TYPE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const a = args[0]!;
    const r = a.ref;
    if (r && (r.c2 > r.c1 || r.r2 > r.r1)) return 64; // rango multi-celda → matriz
    const v = a.value();
    if (isError(v)) return 16;
    if (typeof v === 'number') return 1;
    if (typeof v === 'boolean') return 4;
    return v === '' ? 1 : 2;
  },

  // AREAS(ref): cuántas áreas tiene una referencia. Sin referencias de varias áreas
  // «(A1:B2,D4)», es 1 para toda referencia y #VALUE! para lo que no lo es.
  AREAS(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    return args[0]!.ref ? 1 : ERR.VALUE;
  },

  // FORMULATEXT(ref): la fórmula de la celda tal como se escribió; #N/A si no es una fórmula.
  FORMULATEXT(ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const ref = args[0]!.ref;
    if (!ref) return ERR.VALUE;
    return ctx.formulaText?.(ref) ?? ERR.NA;
  },

  // SHEET([valor]): nº de hoja (1 = la primera) de una referencia o de un nombre de hoja;
  // sin argumentos, el de la hoja de la fórmula. Un nombre que no existe es #N/A.
  SHEET(ctx, args) {
    if (args.length > 1) return ERR.VALUE;
    const nombres = ctx.sheetNames?.() ?? [ctx.here.sheet];
    let hoja = ctx.here.sheet;
    if (args.length === 1) {
      const ref = args[0]!.ref;
      if (ref) hoja = ref.sheet;
      else {
        const v = args[0]!.value();
        if (isError(v)) return v;
        const t = coerceText(v);
        if (isError(t)) return t;
        const n = nombres.findIndex((x) => x.toLowerCase() === t.toLowerCase());
        return n < 0 ? ERR.NA : n + 1;
      }
    }
    const n = nombres.indexOf(hoja);
    return n < 0 ? ERR.NA : n + 1;
  },

  // SHEETS([ref]): cuántas hojas tiene el libro; con una referencia (sin 3D), 1.
  SHEETS(ctx, args) {
    if (args.length > 1) return ERR.VALUE;
    if (args.length === 1) return args[0]!.ref ? 1 : ERR.VALUE;
    return ctx.sheetNames?.().length ?? 1;
  },

  // CELL(tipo, [ref]): información de la celda superior-izquierda de `ref` (por defecto, la
  // de la fórmula). Solo los tipos que no dependen de cómo está formateada la celda:
  // "address", "col", "row", "contents", "type" y "filename" (vacío: el libro no tiene ruta).
  // El resto («format», «color», «prefix», «protect», «width»…) dan #N/A.
  CELL(ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const t0 = coerceText(args[0]!.value());
    if (isError(t0)) return t0;
    const tipo = t0.toLowerCase();
    let ref = args[1]?.ref;
    if (args.length === 2 && !ref) return ERR.VALUE;
    const aqui = ctx.here;
    ref ??= { sheet: aqui.sheet, c1: aqui.col, r1: aqui.row, c2: aqui.col, r2: aqui.row };
    switch (tipo) {
      case 'address':
        return `$${numToCol(ref.c1)}$${ref.r1 + 1}`;
      case 'col':
        return ref.c1 + 1;
      case 'row':
        return ref.r1 + 1;
      case 'filename':
        return '';
      case 'contents':
      case 'type': {
        const v = ctx.resolve({ ...ref, c2: ref.c1, r2: ref.r1 })[0]?.[0] ?? '';
        if (tipo === 'contents') return v;
        return v === '' ? 'b' : typeof v === 'string' ? 'l' : 'v'; // un error cuenta como valor
      }
      default:
        return ERR.NA;
    }
  },

  // ERROR.TYPE: nº de clasificación del error; si NO es un error → #N/A.
  'ERROR.TYPE'(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const v = args[0]!.value();
    if (!isError(v)) return ERR.NA;
    return ERRTYPE[v.code] ?? ERR.NA;
  },
};
