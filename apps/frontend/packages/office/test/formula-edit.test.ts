/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de la UX de edición de fórmulas
 * (sheets/formula-edit.ts): tokenizador con posiciones, identificador/función/
 * argumento bajo el cursor, predicado «¿espera una referencia?», ciclado F4 de
 * anclaje y extracción de referencias + asignación de color. Además valida el
 * troceo de firmas del catálogo (sheets/formula-catalog.ts).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  argHintAt,
  cycleRefAbs,
  cycleRefAt,
  expectsRef,
  extractRefs,
  funcTokenAt,
  refAtCaret,
  tokenizeFx,
} from '../src/editor/sheets/formula-edit';
import { lookupSignature, splitSignature } from '../src/editor/sheets/formula-catalog';

/* ---------- tokenizeFx ---------- */

test('tokenizeFx: posiciones y tipos básicos', () => {
  const toks = tokenizeFx('=SUM(A1;B2)');
  expect(toks.map((t) => `${t.t}:${t.v}@${t.start}`)).toEqual([
    'op:=@0',
    'word:SUM@1',
    'lparen:(@4',
    'word:A1@5',
    'sep:;@7',
    'word:B2@8',
    'rparen:)@10',
  ]);
});

test('tokenizeFx: cadenas y comillas sin cerrar no lanzan', () => {
  expect(() => tokenizeFx('="A1')).not.toThrow();
  const toks = tokenizeFx('="hola"&A1');
  // La cadena abarca las comillas; A1 queda como word tras el «&».
  expect(toks.find((t) => t.t === 'str')?.v).toBe('"hola"');
  expect(toks.find((t) => t.t === 'word')?.v).toBe('A1');
});

/* ---------- funcTokenAt ---------- */

test('funcTokenAt: prefijo del identificador bajo el cursor', () => {
  expect(funcTokenAt('=SU', 3)).toEqual({ prefix: 'SU', start: 1, end: 3 });
  expect(funcTokenAt('=1+AV', 5)).toEqual({ prefix: 'AV', start: 3, end: 5 });
  // Firma con punto (NORM.DIST): el «.» forma parte de la palabra.
  expect(funcTokenAt('=NORM.DI', 8)).toEqual({ prefix: 'NORM.DI', start: 1, end: 8 });
});

test('funcTokenAt: null fuera de una palabra o tras «!»', () => {
  expect(funcTokenAt('=SUM(', 5)).toBeNull(); // cursor tras «(»
  expect(funcTokenAt('=', 1)).toBeNull(); // sin identificador
  expect(funcTokenAt('=Hoja1!A', 8)).toBeNull(); // referencia cualificada por hoja
  expect(funcTokenAt('SUM', 3)).toBeNull(); // no es una fórmula
});

/* ---------- argHintAt ---------- */

test('argHintAt: función envolvente y argumento actual', () => {
  expect(argHintAt('=SUM(A1;B2', 10)).toEqual({ name: 'SUM', argIndex: 1, open: 4 });
  expect(argHintAt('=SUM(A1;B2', 6)).toEqual({ name: 'SUM', argIndex: 0, open: 4 });
  // Anidada: el hint es de la función más interna que rodea al cursor.
  expect(argHintAt('=IF(A1>2;SUM(B1;C1', 18)).toEqual({ name: 'SUM', argIndex: 1, open: 12 });
  // Fuera de la interna, vuelve la externa con SU índice de argumento.
  expect(argHintAt('=IF(A1>2;SUM(B1;C1);', 20)).toEqual({ name: 'IF', argIndex: 2, open: 3 });
});

test('argHintAt: null sin función envolvente', () => {
  expect(argHintAt('=1+2', 4)).toBeNull();
  expect(argHintAt('=(A1+B1)', 5)).toBeNull(); // paréntesis de agrupación, no llamada
});

/* ---------- expectsRef ---------- */

test('expectsRef: posiciones que esperan un operando/referencia', () => {
  expect(expectsRef('=', 1)).toBe(true); // tras «=»
  expect(expectsRef('=A1+', 4)).toBe(true); // tras un operador
  expect(expectsRef('=SUM(', 5)).toBe(true); // tras «(»
  expect(expectsRef('=SUM(A1;', 8)).toBe(true); // tras un separador
});

test('expectsRef: posiciones que NO esperan referencia', () => {
  expect(expectsRef('=A1', 3)).toBe(false); // justo tras una referencia
  expect(expectsRef('=SUM(A1', 7)).toBe(false); // idem dentro de la llamada
  expect(expectsRef('=1+2', 4)).toBe(false); // tras un número
  expect(expectsRef('hola', 4)).toBe(false); // no es fórmula
});

/* ---------- cycleRefAbs (F4) ---------- */

test('cycleRefAbs: ciclo A1 → $A$1 → A$1 → $A1 → A1', () => {
  expect(cycleRefAbs('A1')).toBe('$A$1');
  expect(cycleRefAbs('$A$1')).toBe('A$1');
  expect(cycleRefAbs('A$1')).toBe('$A1');
  expect(cycleRefAbs('$A1')).toBe('A1');
});

test('cycleRefAbs: conserva hoja y aplica el mismo estado a un rango', () => {
  expect(cycleRefAbs('Hoja1!A1')).toBe('Hoja1!$A$1');
  expect(cycleRefAbs('A1:B2')).toBe('$A$1:$B$2');
  expect(cycleRefAbs('$A$1:$B$2')).toBe('A$1:B$2');
  expect(cycleRefAbs('no-es-ref')).toBe('no-es-ref');
});

test('cycleRefAt: cicla la referencia bajo el cursor dentro del texto', () => {
  expect(cycleRefAt('=A1+B2', 2)).toEqual({ text: '=$A$1+B2', caret: 5 });
  expect(cycleRefAt('=A1+B2', 5)).toEqual({ text: '=A1+$B$2', caret: 8 });
  expect(cycleRefAt('=1+2', 2)).toBeNull(); // el cursor no está sobre una ref
});

/* ---------- extractRefs + color ---------- */

test('extractRefs: posiciones, rects y color estable por ref distinta', () => {
  const refs = extractRefs('=A1+B2*A1');
  expect(refs.map((r) => `${r.text}@${r.start}:${r.end}#${r.colorIndex}`)).toEqual([
    'A1@1:3#0',
    'B2@4:6#1',
    'A1@7:9#0', // misma celda que la primera → mismo color
  ]);
  expect(refs[0]!.rect).toEqual({ c1: 0, r1: 0, c2: 0, r2: 0 });
});

test('extractRefs: el anclaje no cambia el color (misma zona)', () => {
  const refs = extractRefs('=A1+$A$1');
  expect(refs.map((r) => r.colorIndex)).toEqual([0, 0]);
});

test('extractRefs: rangos, hojas, cadenas y nombres de función', () => {
  const range = extractRefs('=SUM(A1:B3)');
  expect(range).toHaveLength(1);
  expect(range[0]!.text).toBe('A1:B3');
  expect(range[0]!.rect).toEqual({ c1: 0, r1: 0, c2: 1, r2: 2 });

  const qualified = extractRefs('=Hoja2!A1+A1');
  expect(qualified.map((r) => `${r.text}#${r.colorIndex}`)).toEqual(['Hoja2!A1#0', 'A1#1']);
  expect(qualified[0]!.sheet).toBe('Hoja2');

  expect(extractRefs('="A1"+B1').map((r) => r.text)).toEqual(['B1']); // A1 dentro de comillas se ignora
  expect(extractRefs('=LOG10(2)+A1').map((r) => r.text)).toEqual(['A1']); // LOG10( es función
});

/* ---------- extractRefs: columnas/filas enteras (A:A, 1:1) ---------- */

test('extractRefs: columna entera A:A y A:C', () => {
  const aa = extractRefs('=SUM(A:A)');
  expect(aa).toHaveLength(1);
  expect(aa[0]!.text).toBe('A:A');
  expect(aa[0]!.rect.c1).toBe(0);
  expect(aa[0]!.rect.c2).toBe(0);
  expect(aa[0]!.rect.r1).toBe(0);
  expect(aa[0]!.rect.r2).toBeGreaterThan(1000); // eje libre: toda la columna

  const ac = extractRefs('=SUM(A:C)');
  expect(ac[0]!.text).toBe('A:C');
  expect(ac[0]!.rect).toMatchObject({ c1: 0, c2: 2, r1: 0 });
});

test('extractRefs: fila entera 1:1 y 3:5, con hoja y absolutas', () => {
  const one = extractRefs('=SUM(1:1)');
  expect(one[0]!.text).toBe('1:1');
  expect(one[0]!.rect).toMatchObject({ r1: 0, r2: 0, c1: 0 });
  expect(one[0]!.rect.c2).toBeGreaterThan(1000);

  const r35 = extractRefs('=SUM(3:5)');
  expect(r35[0]!.text).toBe('3:5');
  expect(r35[0]!.rect).toMatchObject({ r1: 2, r2: 4, c1: 0 });

  expect(extractRefs('=SUM($A:$B)')[0]!.text).toBe('$A:$B');
  const q = extractRefs('=SUM(Hoja1!A:A)');
  expect(q[0]!.text).toBe('Hoja1!A:A');
  expect(q[0]!.sheet).toBe('Hoja1');
});

test('extractRefs: una columna/fila SUELTA no es referencia', () => {
  expect(extractRefs('=A+1')).toHaveLength(0); // «A» sola no es ref
  expect(extractRefs('=3+A1').map((r) => r.text)).toEqual(['A1']); // «3» no es ref
});

test('refAtCaret: sobre una columna/fila entera (para sustituirla al clicar)', () => {
  expect(refAtCaret('=SUM(A:A)', 7)?.text).toBe('A:A'); // cursor dentro de A:A
  expect(refAtCaret('=SUM(1:3)', 8)?.text).toBe('1:3');
});

/* ---------- catálogo de firmas ---------- */

test('splitSignature: nombre y parámetros', () => {
  expect(splitSignature('SUM(número1; [número2]; …)')).toEqual({
    name: 'SUM',
    params: ['número1', '[número2]', '…'],
  });
  expect(splitSignature('PI()')).toEqual({ name: 'PI', params: [] });
});

test('lookupSignature: insensible a mayúsculas; ausente = undefined', () => {
  expect(lookupSignature('vlookup')?.args).toContain('VLOOKUP(');
  expect(lookupSignature('NoExiste')).toBeUndefined();
});

/* ---------- refAtCaret (point-and-click: sustituir la ref bajo el cursor) ---------- */

test('refAtCaret: la ref que toca el cursor (inclusive en ambos extremos)', () => {
  // «=A1», cursor al FINAL de A1 (justo tras autocompletarla con un clic): la toca.
  expect(refAtCaret('=A1', 3)?.text).toBe('A1');
  // cursor al INICIO de A1 y en medio: también.
  expect(refAtCaret('=A1', 1)?.text).toBe('A1');
  expect(refAtCaret('=A1', 2)?.text).toBe('A1');
  // «=A1+B2»: el offset 3 (fin de A1 / antes del «+») → A1; dentro de B2 → B2.
  expect(refAtCaret('=A1+B2', 3)?.text).toBe('A1');
  expect(refAtCaret('=A1+B2', 5)?.text).toBe('B2');
  // rango: el cursor en cualquier punto de A1:B2 → todo el rango.
  expect(refAtCaret('=SUM(A1:B2)', 7)?.text).toBe('A1:B2');
  expect(refAtCaret('=SUM(A1:B2)', 10)?.text).toBe('A1:B2');
  // sin ref bajo el cursor / no es fórmula → null.
  expect(refAtCaret('=1+2', 2)).toBeNull();
  expect(refAtCaret('=A1', 0)).toBeNull(); // sobre el «=», no la ref
  expect(refAtCaret('A1', 1)).toBeNull(); // no empieza por «=»
});
