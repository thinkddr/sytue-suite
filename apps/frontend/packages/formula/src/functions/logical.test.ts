// Checks de las funciones lógicas. Valores conocidos de una hoja real, casos de
// error y pruebas de pereza (la rama no tomada NO debe evaluarse). `bun test`.
import { expect, test } from 'bun:test';
import { funcs } from './logical.js';
import { Engine } from '../engine.js';
import { ERR, isError, type CellValue } from '../values.js';
import type { Arg, FnCtx } from '../registry.js';

// Ninguna función lógica usa el ctx; basta un ctx mínimo válido.
const ctx: FnCtx = {
  now: () => new Date(0),
  random: () => 0,
  here: { sheet: 'S', col: 0, row: 0 },
  resolve: () => [],
  parseRef: () => undefined,
  isFormula: () => false,
  resolveTable: () => undefined,
};

/** Arg escalar con un valor fijo (referencia = ninguna). */
function arg(v: CellValue): Arg {
  return { value: () => v, matrix: () => [[v]], ref: undefined };
}

/** Arg de rango: matriz de celdas y `ref` definido (para el trato «rango»). */
function rangeArg(m: CellValue[][]): Arg {
  return {
    value: () => (m.length === 1 && m[0]!.length === 1 ? m[0]![0]! : ERR.VALUE),
    matrix: () => m,
    ref: { sheet: 'S', c1: 0, r1: 0, c2: (m[0]?.length ?? 1) - 1, r2: m.length - 1 },
  };
}

/** Arg que estalla si se evalúa: prueba que una rama NO se toca (pereza). */
function boom(): Arg {
  const bang = (): never => {
    throw new Error('no debía evaluarse');
  };
  return { value: bang, matrix: bang, ref: undefined };
}

const call = (name: keyof typeof funcs, args: Arg[]): CellValue =>
  funcs[name]!(ctx, args) as CellValue;

/* ---------- IF ---------- */

test('IF: rama tomada y valor por defecto', () => {
  expect(call('IF', [arg(true), arg(1), arg(2)])).toBe(1);
  expect(call('IF', [arg(false), arg(1), arg(2)])).toBe(2);
  expect(call('IF', [arg(false), arg(1)])).toBe(false); // sin «si no» → FALSE
  expect(call('IF', [arg(0), arg('a'), arg('b')])).toBe('b'); // 0 → falso
  expect(call('IF', [arg('TRUE'), arg('sí'), arg('no')])).toBe('sí'); // texto → bool
  // Convención blank≈"" (núcleo, decisión #4): cadena vacía → FALSE. Excel daría
  // #VALUE! con "" LITERAL, pero aquí "" es indistinguible de celda en blanco.
  expect(call('IF', [arg(''), arg('a'), arg('b')])).toBe('b');
});

test('IF: es perezoso (la rama no tomada no se evalúa)', () => {
  expect(call('IF', [arg(true), arg(1), boom()])).toBe(1);
  expect(call('IF', [arg(false), boom(), arg(2)])).toBe(2);
});

test('IF: condición errónea o mal nº de argumentos → error', () => {
  expect(call('IF', [arg(ERR.DIV0), arg(1), arg(2)])).toBe(ERR.DIV0);
  expect(call('IF', [arg(true)])).toBe(ERR.VALUE); // faltan argumentos
  expect(call('IF', [arg(true), arg(1), arg(2), arg(3)])).toBe(ERR.VALUE);
});

/* ---------- IFS ---------- */

test('IFS: primera condición verdadera gana; #N/A si ninguna', () => {
  expect(call('IFS', [arg(false), arg('a'), arg(true), arg('b')])).toBe('b');
  expect(call('IFS', [arg(true), arg('a'), arg(true), arg('b')])).toBe('a');
  expect(call('IFS', [arg(false), arg('a'), arg(false), arg('b')])).toBe(ERR.NA);
  // No evalúa el resultado de una condición falsa (pereza).
  expect(call('IFS', [arg(false), boom(), arg(true), arg('ok')])).toBe('ok');
  // Tampoco evalúa CONDICIONES posteriores tras la primera verdadera (pereza total).
  expect(call('IFS', [arg(true), arg('a'), boom(), arg('b')])).toBe('a');
});

/* ---------- AND / OR / XOR ---------- */

test('AND: verdadero si todos; propaga error (eager)', () => {
  expect(call('AND', [arg(true), arg(true), arg(1)])).toBe(true);
  expect(call('AND', [arg(true), arg(false)])).toBe(false);
  expect(call('AND', [arg('TRUE'), arg('FALSE')])).toBe(false);
  // Eager: aunque el primero sea falso, evalúa el resto y propaga el error.
  expect(call('AND', [arg(false), arg(ERR.DIV0)])).toBe(ERR.DIV0);
  expect(call('AND', [])).toBe(ERR.VALUE); // sin valores lógicos
});

test('OR: verdadero si alguno; propaga error (eager)', () => {
  expect(call('OR', [arg(false), arg(false)])).toBe(false);
  expect(call('OR', [arg(false), arg(true)])).toBe(true);
  expect(call('OR', [arg(true), arg(ERR.NA)])).toBe(ERR.NA); // eager
});

test('XOR: verdadero si un nº impar es verdadero', () => {
  expect(call('XOR', [arg(true), arg(false)])).toBe(true);
  expect(call('XOR', [arg(true), arg(true)])).toBe(false);
  expect(call('XOR', [arg(true), arg(true), arg(true)])).toBe(true);
  expect(call('XOR', [arg(1), arg(0)])).toBe(true);
  // Eager como AND/OR: propaga el error aunque el resultado de paridad no lo necesite.
  expect(call('XOR', [arg(true), arg(ERR.DIV0)])).toBe(ERR.DIV0);
  expect(call('XOR', [])).toBe(ERR.VALUE); // sin valores lógicos
});

test('AND/OR/XOR sobre rango: ignoran texto y blancos', () => {
  // Rango [TRUE, 1, "x", "" ] → solo cuentan TRUE y 1.
  expect(call('AND', [rangeArg([[true], [1], ['x'], ['']])])).toBe(true);
  expect(call('AND', [rangeArg([[true], [0]])])).toBe(false); // 0 → falso
  expect(call('OR', [rangeArg([[false], ['texto'], [1]])])).toBe(true);
  // Rango solo con texto/blancos → sin valores lógicos → #VALUE!.
  expect(call('AND', [rangeArg([['x'], ['']])])).toBe(ERR.VALUE);
});

/* ---------- NOT ---------- */

test('NOT: niega; coacciona número/texto; propaga error', () => {
  expect(call('NOT', [arg(true)])).toBe(false);
  expect(call('NOT', [arg(false)])).toBe(true);
  expect(call('NOT', [arg(0)])).toBe(true); // 0 → falso → NOT → true
  expect(call('NOT', [arg(5)])).toBe(false);
  expect(call('NOT', [arg('hola')])).toBe(ERR.VALUE); // texto no lógico
  expect(call('NOT', [arg(ERR.DIV0)])).toBe(ERR.DIV0);
  expect(call('NOT', [])).toBe(ERR.VALUE); // nº de argumentos
});

/* ---------- TRUE / FALSE ---------- */

test('TRUE()/FALSE(): constantes', () => {
  expect(call('TRUE', [])).toBe(true);
  expect(call('FALSE', [])).toBe(false);
});

/* ---------- IFERROR / IFNA ---------- */

test('IFERROR: captura cualquier error; si no, pasa el valor', () => {
  expect(call('IFERROR', [arg(ERR.DIV0), arg('reserva')])).toBe('reserva');
  expect(call('IFERROR', [arg(ERR.NA), arg('reserva')])).toBe('reserva'); // también #N/A
  expect(call('IFERROR', [arg(5), arg('reserva')])).toBe(5);
  expect(call('IFERROR', [arg(5), boom()])).toBe(5); // perezoso: no evalúa la reserva
  expect(call('IFERROR', [arg(1)])).toBe(ERR.VALUE); // nº de argumentos
  // Si la reserva es a su vez un error, se devuelve ESE error (el de la reserva).
  expect(call('IFERROR', [arg(ERR.DIV0), arg(ERR.NA)])).toBe(ERR.NA);
});

test('IFNA: captura SOLO #N/A; otros errores se propagan', () => {
  expect(call('IFNA', [arg(ERR.NA), arg('reserva')])).toBe('reserva');
  expect(call('IFNA', [arg(ERR.DIV0), arg('reserva')])).toBe(ERR.DIV0); // NO se captura
  expect(call('IFNA', [arg(7), arg('reserva')])).toBe(7);
  expect(call('IFNA', [arg(7), boom()])).toBe(7); // perezoso
});

/* ---------- SWITCH ---------- */

test('SWITCH: empareja valor; predeterminado; #N/A si nada', () => {
  const cases = [arg(1), arg('uno'), arg(2), arg('dos'), arg('otro')];
  expect(call('SWITCH', [arg(2), ...cases])).toBe('dos');
  expect(call('SWITCH', [arg(1), ...cases])).toBe('uno');
  expect(call('SWITCH', [arg(9), ...cases])).toBe('otro'); // predeterminado
  // Sin predeterminado y sin emparejar → #N/A.
  expect(call('SWITCH', [arg(9), arg(1), arg('uno'), arg(2), arg('dos')])).toBe(ERR.NA);
  // Texto insensible a mayúsculas (igualdad de hoja).
  expect(call('SWITCH', [arg('a'), arg('A'), arg('match'), arg('no')])).toBe('match');
  // Número y texto-numérico NO son iguales (como el operador «=» de hoja): 1 ≠ "1".
  expect(call('SWITCH', [arg(1), arg('1'), arg('x'), arg('def')])).toBe('def');
});

test('SWITCH: un error en un valor evaluado antes del match se propaga', () => {
  // Los valores se comparan de izq. a der.; el 1.º da error → se propaga (eager en valores).
  expect(call('SWITCH', [arg(2), arg(ERR.DIV0), arg('a'), arg(2), arg('b')])).toBe(ERR.DIV0);
});

test('SWITCH: perezoso (no evalúa resultados ni predeterminado no usados)', () => {
  // Empareja el 2º caso: el resultado del 1º (boom) no se evalúa.
  expect(call('SWITCH', [arg(2), arg(1), boom(), arg(2), arg('b')])).toBe('b');
  // Empareja un caso: el predeterminado (boom) no se evalúa.
  expect(call('SWITCH', [arg(1), arg(1), arg('a'), boom()])).toBe('a');
});

test('SWITCH: propaga error de la expresión', () => {
  expect(call('SWITCH', [arg(ERR.DIV0), arg(1), arg('a'), arg('def')])).toBe(ERR.DIV0);
});

/* ---------- Integración end-to-end vía Engine ---------- */

test('Engine: IF/AND/OR/IFERROR con celdas y display', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '10');
  e.setCell('Hoja1', 'A2', '=IF(A1>5, "grande", "pequeño")');
  expect(e.getValue('Hoja1', 'A2')).toBe('grande');

  // AND/OR sobre un rango con texto intercalado (se ignora).
  e.setCell('Hoja1', 'B1', 'TRUE');
  e.setCell('Hoja1', 'B2', '1');
  e.setCell('Hoja1', 'B3', 'texto'); // ignorado en el rango
  e.setCell('Hoja1', 'C1', '=AND(B1:B3)');
  expect(e.getValue('Hoja1', 'C1')).toBe(true);

  // AND es eager: propaga el #DIV/0! aunque el primer argumento sea falso.
  e.setCell('Hoja1', 'C2', '=AND(FALSE, 1/0)');
  expect(e.getDisplay('Hoja1', 'C2')).toBe('#DIV/0!');

  // IFERROR captura el error.
  e.setCell('Hoja1', 'C3', '=IFERROR(1/0, "err")');
  expect(e.getValue('Hoja1', 'C3')).toBe('err');

  // IFERROR también captura #NAME? (función inexistente), uso real habitual.
  e.setCell('Hoja1', 'C4', '=IFERROR(NOEXISTE(), "cazado")');
  expect(e.getValue('Hoja1', 'C4')).toBe('cazado');
});

test('Engine: IF es perezoso (no propaga error de la rama no tomada)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=IF(TRUE, 1, 1/0)');
  expect(e.getValue('Hoja1', 'A1')).toBe(1);
});

test('Engine: IFS y SWITCH', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '85');
  e.setCell('Hoja1', 'A2', '=IFS(A1>=90,"A",A1>=80,"B",A1>=70,"C",TRUE,"F")');
  expect(e.getValue('Hoja1', 'A2')).toBe('B');
  e.setCell('Hoja1', 'A3', '=SWITCH(3, 1, "uno", 2, "dos", "otro")');
  expect(e.getValue('Hoja1', 'A3')).toBe('otro');
});

// Verificación de que los errores devueltos son FormulaError (no strings).
test('sanidad: los errores son FormulaError', () => {
  expect(isError(call('NOT', [arg('x')]))).toBe(true);
});
