// Checks de las funciones de información (IS*, N, TYPE, ERROR.TYPE) contra
// valores conocidos de una hoja real, con casos de error. `bun test`.
import { expect, test } from 'bun:test';
import { funcs } from './information.js';
import { Engine } from '../engine.js';
import { ERR, type CellValue } from '../values.js';
import type { Arg, FnCtx } from '../registry.js';

// Ninguna función de información usa el ctx (TYPE mira arg.ref, no el ctx).
const ctx: FnCtx = {
  now: () => new Date(0),
  random: () => 0,
  here: { sheet: 'S', col: 0, row: 0 },
  resolve: () => [],
  parseRef: () => undefined,
  isFormula: () => false,
  resolveTable: () => undefined,
};

/** Arg escalar con un valor fijo (sin referencia). */
function arg(v: CellValue): Arg {
  return { value: () => v, matrix: () => [[v]], ref: undefined };
}

/** Arg de rango: matriz de celdas y `ref` definido (para el trato «matriz» de TYPE). */
function rangeArg(m: CellValue[][]): Arg {
  return {
    value: () => (m.length === 1 && m[0]!.length === 1 ? m[0]![0]! : ERR.VALUE),
    matrix: () => m,
    ref: { sheet: 'S', c1: 0, r1: 0, c2: (m[0]?.length ?? 1) - 1, r2: m.length - 1 },
  };
}

const call = (name: keyof typeof funcs, args: Arg[]): CellValue =>
  funcs[name]!(ctx, args) as CellValue;

/** Evalúa `formula` en Z1 (motor nuevo) con `cells` fijadas antes. Valor calculado. */
function val(formula: string, cells: Record<string, string> = {}): CellValue {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}

/** Igual que `val` pero devuelve el texto a mostrar (para errores). */
function disp(formula: string, cells: Record<string, string> = {}): string {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

/* ---------- IS* de tipo (no coaccionan, no propagan) ---------- */

test('ISNUMBER / ISTEXT / ISNONTEXT / ISLOGICAL', () => {
  expect(call('ISNUMBER', [arg(42)])).toBe(true);
  expect(call('ISNUMBER', [arg('42')])).toBe(false); // texto, aunque parezca número
  expect(call('ISNUMBER', [arg(true)])).toBe(false);
  expect(call('ISNUMBER', [arg(ERR.DIV0)])).toBe(false); // no propaga

  expect(call('ISTEXT', [arg('hola')])).toBe(true);
  expect(call('ISTEXT', [arg(42)])).toBe(false);
  expect(call('ISTEXT', [arg(true)])).toBe(false);
  expect(call('ISTEXT', [arg('')])).toBe(false); // blanco no es texto
  expect(call('ISTEXT', [arg(ERR.NA)])).toBe(false); // error no es texto (no propaga)

  expect(call('ISNONTEXT', [arg(42)])).toBe(true);
  expect(call('ISNONTEXT', [arg('hola')])).toBe(false);
  expect(call('ISNONTEXT', [arg('')])).toBe(true); // blanco es no-texto (regla de Excel)
  expect(call('ISNONTEXT', [arg(ERR.NA)])).toBe(true);

  expect(call('ISLOGICAL', [arg(true)])).toBe(true);
  expect(call('ISLOGICAL', [arg(false)])).toBe(true);
  expect(call('ISLOGICAL', [arg(1)])).toBe(false);
  expect(call('ISLOGICAL', [arg('TRUE')])).toBe(false); // texto, no lógico
});

test('ISBLANK: solo el valor en blanco es TRUE', () => {
  expect(call('ISBLANK', [arg('')])).toBe(true);
  expect(call('ISBLANK', [arg(0)])).toBe(false);
  expect(call('ISBLANK', [arg('x')])).toBe(false);
  expect(call('ISBLANK', [arg(false)])).toBe(false);
  expect(call('ISBLANK', [arg(ERR.NA)])).toBe(false); // un error no es blanco
});

test('ISERROR / ISERR / ISNA: clasifican, no propagan', () => {
  expect(call('ISERROR', [arg(ERR.DIV0)])).toBe(true);
  expect(call('ISERROR', [arg(ERR.NA)])).toBe(true); // ISERROR SÍ incluye #N/A
  expect(call('ISERROR', [arg(5)])).toBe(false);
  expect(call('ISERROR', [arg('#DIV/0!')])).toBe(false); // texto que parece error, no lo es

  expect(call('ISERR', [arg(ERR.DIV0)])).toBe(true);
  expect(call('ISERR', [arg(ERR.NA)])).toBe(false); // ISERR EXCLUYE #N/A
  expect(call('ISERR', [arg('x')])).toBe(false);

  expect(call('ISNA', [arg(ERR.NA)])).toBe(true);
  expect(call('ISNA', [arg(ERR.DIV0)])).toBe(false);
  expect(call('ISNA', [arg(5)])).toBe(false);
});

test('ISREF: TRUE solo si el argumento es referencia/rango (mira la forma, no el valor)', () => {
  expect(call('ISREF', [rangeArg([[5]])])).toBe(true); // A1
  expect(
    call('ISREF', [
      rangeArg([
        [1, 2],
        [3, 4],
      ]),
    ]),
  ).toBe(true); // A1:B2
  expect(call('ISREF', [arg(5)])).toBe(false); // número
  expect(call('ISREF', [arg('A1')])).toBe(false); // texto con pinta de ref, no lo es
  expect(call('ISREF', [arg(ERR.NA)])).toBe(false); // un error no es referencia
  expect(call('ISREF', [])).toBe(ERR.VALUE);
  expect(call('ISREF', [arg(1), arg(2)])).toBe(ERR.VALUE);
});

test('IS*: nº de argumentos incorrecto → #VALUE!', () => {
  expect(call('ISNUMBER', [])).toBe(ERR.VALUE);
  expect(call('ISBLANK', [arg(1), arg(2)])).toBe(ERR.VALUE);
  expect(call('N', [])).toBe(ERR.VALUE);
  expect(call('TYPE', [])).toBe(ERR.VALUE);
});

/* ---------- ISEVEN / ISODD (coaccionan y propagan) ---------- */

test('ISEVEN / ISODD: truncan hacia cero; blanco → 0; propagan error', () => {
  expect(call('ISEVEN', [arg(4)])).toBe(true);
  expect(call('ISEVEN', [arg(3)])).toBe(false);
  expect(call('ISEVEN', [arg(2.5)])).toBe(true); // trunca 2,5 → 2 (par)
  expect(call('ISEVEN', [arg(0)])).toBe(true);
  expect(call('ISEVEN', [arg(-2)])).toBe(true);
  expect(call('ISEVEN', [arg(-3.5)])).toBe(false); // trunca HACIA CERO → -3 (impar), no -4
  expect(call('ISEVEN', [arg(-2.9)])).toBe(true); // trunca → -2 (par)
  expect(call('ISEVEN', [arg('')])).toBe(true); // blanco → 0 → par
  expect(call('ISEVEN', [arg('6')])).toBe(true); // texto numérico se coacciona
  expect(call('ISEVEN', [arg('x')])).toBe(ERR.VALUE); // texto no numérico
  expect(call('ISEVEN', [arg(ERR.DIV0)])).toBe(ERR.DIV0); // propaga

  expect(call('ISODD', [arg(3)])).toBe(true);
  expect(call('ISODD', [arg(4)])).toBe(false);
  expect(call('ISODD', [arg(3.5)])).toBe(true); // trunca 3,5 → 3 (impar)
  expect(call('ISODD', [arg(0)])).toBe(false);
  expect(call('ISODD', [arg(-3)])).toBe(true);
  expect(call('ISODD', [arg(-3.5)])).toBe(true); // trunca HACIA CERO → -3 (impar), no -4
  expect(call('ISODD', [arg(-2.9)])).toBe(false); // trunca → -2 (par)
  expect(call('ISODD', [arg('x')])).toBe(ERR.VALUE);
});

/* ---------- NA / N ---------- */

test('NA(): constante #N/A', () => {
  expect(call('NA', [])).toBe(ERR.NA);
});

test('N: números y booleanos; texto → 0 (sin parsear); propaga error', () => {
  expect(call('N', [arg(7)])).toBe(7);
  expect(call('N', [arg(-3.5)])).toBe(-3.5);
  expect(call('N', [arg(true)])).toBe(1);
  expect(call('N', [arg(false)])).toBe(0);
  expect(call('N', [arg('7')])).toBe(0); // texto NO se parsea (a diferencia de la coacción)
  expect(call('N', [arg('hola')])).toBe(0);
  expect(call('N', [arg('')])).toBe(0);
  expect(call('N', [arg(ERR.DIV0)])).toBe(ERR.DIV0);
});

/* ---------- TYPE ---------- */

test('TYPE: 1 número, 2 texto, 4 lógico, 16 error, 64 matriz', () => {
  expect(call('TYPE', [arg(42)])).toBe(1);
  expect(call('TYPE', [arg('hola')])).toBe(2);
  expect(call('TYPE', [arg(true)])).toBe(4);
  expect(call('TYPE', [arg(ERR.DIV0)])).toBe(16); // clasifica el error, no lo propaga
  expect(call('TYPE', [arg('')])).toBe(1); // blanco cuenta como número
  expect(call('TYPE', [rangeArg([[5]])])).toBe(1); // rango 1×1 → su valor escalar
  expect(
    call('TYPE', [
      rangeArg([
        [1, 2],
        [3, 4],
      ]),
    ]),
  ).toBe(64); // rango multi-celda → matriz
});

/* ---------- ERROR.TYPE ---------- */

test('ERROR.TYPE: nº de cada error; #N/A si no es error', () => {
  expect(call('ERROR.TYPE', [arg(ERR.NULL)])).toBe(1);
  expect(call('ERROR.TYPE', [arg(ERR.DIV0)])).toBe(2);
  expect(call('ERROR.TYPE', [arg(ERR.VALUE)])).toBe(3);
  expect(call('ERROR.TYPE', [arg(ERR.REF)])).toBe(4);
  expect(call('ERROR.TYPE', [arg(ERR.NAME)])).toBe(5);
  expect(call('ERROR.TYPE', [arg(ERR.NUM)])).toBe(6);
  expect(call('ERROR.TYPE', [arg(ERR.NA)])).toBe(7);
  expect(call('ERROR.TYPE', [arg(ERR.CIRC)])).toBe(4); // #CIRC! ≈ #REF! circular
  expect(call('ERROR.TYPE', [arg(5)])).toBe(ERR.NA); // no es error → #N/A
  expect(call('ERROR.TYPE', [arg('texto')])).toBe(ERR.NA);
});

/* ---------- Integración end-to-end vía Engine ---------- */

test('Engine: ISBLANK/ISNUMBER/ISTEXT con celda vacía vs llena', () => {
  expect(val('=ISBLANK(A3)')).toBe(true); // A3 nunca escrita → en blanco
  expect(val('=ISBLANK(A1)', { A1: '10' })).toBe(false);
  expect(val('=ISNUMBER(A1)', { A1: '10' })).toBe(true);
  expect(val('=ISTEXT(A2)', { A2: 'hola' })).toBe(true);
  expect(val('=ISTEXT(A3)')).toBe(false); // blanco no es texto
  expect(val('=ISNONTEXT(A3)')).toBe(true);
});

test('Engine: ISERROR/ISERR/ISNA con errores producidos por fórmulas', () => {
  expect(val('=ISERROR(1/0)')).toBe(true);
  expect(val('=ISERR(1/0)')).toBe(true);
  expect(val('=ISERR(NA())')).toBe(false); // #N/A excluido de ISERR
  expect(val('=ISNA(NA())')).toBe(true);
  expect(val('=ISNUMBER(1/0)')).toBe(false); // inspecciona, no propaga
  // Patrón típico: envolver una división que puede fallar.
  expect(val('=IF(ISERROR(A1/A2),"err",A1/A2)', { A1: '10', A2: '0' })).toBe('err');
  expect(val('=IF(ISERROR(A1/A2),"err",A1/A2)', { A1: '10', A2: '5' })).toBe(2);
});

test('Engine: TYPE y ERROR.TYPE end-to-end (nombre con punto incluido)', () => {
  expect(val('=TYPE(A1)', { A1: '42' })).toBe(1);
  expect(val('=TYPE(A1)', { A1: 'hola' })).toBe(2);
  expect(val('=TYPE(A1)')).toBe(1); // celda en blanco → número
  expect(val('=TYPE(A1:B2)', { A1: '1', B2: '2' })).toBe(64); // rango multi-celda
  expect(val('=ERROR.TYPE(1/0)')).toBe(2);
  expect(val('=ERROR.TYPE(NA())')).toBe(7);
  expect(val('=ERROR.TYPE(SQRT(-1))')).toBe(6); // #NUM!
  expect(disp('=ERROR.TYPE(5)')).toBe('#N/A'); // no es error
});

test('Engine: ISREF distingue referencias de valores; no evalúa el argumento', () => {
  expect(val('=ISREF(A1)')).toBe(true);
  expect(val('=ISREF(A1:B2)')).toBe(true);
  expect(val('=ISREF(Hoja1!A1)')).toBe(true); // referencia con hoja
  expect(val('=ISREF(5)')).toBe(false);
  expect(val('=ISREF("A1")')).toBe(false); // texto, no referencia
  expect(val('=ISREF(SUM(A1:A2))')).toBe(false); // el resultado de una llamada no es ref
  expect(val('=ISREF(1/0)')).toBe(false); // inspecciona la forma → NO propaga #DIV/0!
});

test('Engine: ISEVEN/ISODD con celdas; blanco → 0; texto → #VALUE!', () => {
  expect(val('=ISEVEN(A1)', { A1: '4' })).toBe(true);
  expect(val('=ISODD(A1)', { A1: '4' })).toBe(false);
  expect(val('=ISEVEN(A1)')).toBe(true); // A1 en blanco → 0 → par
  expect(disp('=ISEVEN(A1)', { A1: 'hola' })).toBe('#VALUE!');
});

test('Engine: referencia circular → #CIRC!, clasificada por ERROR.TYPE como 4', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=A1'); // ciclo directo
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#CIRC!');
  e.setCell('Hoja1', 'B1', '=ISERROR(A1)');
  expect(e.getValue('Hoja1', 'B1')).toBe(true);
  e.setCell('Hoja1', 'C1', '=ERROR.TYPE(A1)');
  expect(e.getValue('Hoja1', 'C1')).toBe(4);
});
