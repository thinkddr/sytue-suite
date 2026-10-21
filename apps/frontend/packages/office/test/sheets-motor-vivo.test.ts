/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del motor vivo de Calcus (sheets/motor-vivo.ts): el motor se conserva entre
 * ediciones y se le aplica solo la diferencia; si cambia la estructura, se reconstruye.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import type { SheetData, SheetModel } from '../src/editor/types';
import { anotarCambio, construirMotor, sincronizarMotor } from '../src/editor/sheets/motor-vivo';

const H = 'Hoja1';

function modelo(cells: Record<string, string>, extra: Partial<SheetData> = {}): SheetModel {
  return { active: 0, sheets: [{ name: H, cells, ...extra }] };
}

/** El modelo con una celda cambiada (inmutable, como hace el editor). */
function con(m: SheetModel, ref: string, html: string): SheetModel {
  const cells = { ...m.sheets[0]!.cells };
  if (html === '') delete cells[ref];
  else cells[ref] = html;
  return { ...m, sheets: [{ ...m.sheets[0]!, cells }, ...m.sheets.slice(1)] };
}

test('una edición se aplica al mismo motor y solo reevalúa lo que depende de ella', () => {
  const m0 = modelo({ A1: '1', A2: '2', B1: '=A1*10', B2: '=A2*10', C1: '=SUM(B1:B2)' });
  const v0 = construirMotor(m0);
  v0.engine.recalc();
  const base = v0.engine.evaluaciones;

  const { vivo, escritas } = sincronizarMotor(v0, con(m0, 'A1', '5'));
  expect(vivo).toBe(v0); // la MISMA instancia
  expect(vivo.engine).toBe(v0.engine);
  expect(escritas).toBe(1);
  expect(vivo.engine.getValue(H, 'C1')).toBe(70); // 5·10 + 2·10
  // B1 y C1; B2 sigue en caché.
  expect(vivo.engine.evaluaciones - base).toBe(2);
});

test('el mismo modelo no escribe nada', () => {
  const m0 = modelo({ A1: '1' });
  const v0 = construirMotor(m0);
  expect(sincronizarMotor(v0, m0).escritas).toBe(0);
});

test('cambiar solo el formato (negrita) no toca el motor', () => {
  const m0 = modelo({ A1: '5', B1: '=A1+1' });
  const v0 = construirMotor(m0);
  v0.engine.recalc();
  const base = v0.engine.evaluaciones;
  const { escritas } = sincronizarMotor(v0, con(m0, 'A1', '<b>5</b>'));
  expect(escritas).toBe(0);
  v0.engine.recalc();
  expect(v0.engine.evaluaciones).toBe(base);
});

test('borrar una celda la vacía en el motor y recalcula a sus dependientes', () => {
  const m0 = modelo({ A1: '4', B1: '=A1*2' });
  const v0 = construirMotor(m0);
  expect(v0.engine.getValue(H, 'B1')).toBe(8);
  const { escritas } = sincronizarMotor(v0, con(m0, 'A1', ''));
  expect(escritas).toBe(1);
  expect(v0.engine.getRaw(H, 'A1')).toBe('');
  expect(v0.engine.getValue(H, 'B1')).toBe(0);
});

test('equivale a construir de cero tras una secuencia de ediciones', () => {
  let m = modelo({ A1: '1', A2: '2', A3: '=A1+A2', B1: '=SUM(A1:A3)', B2: '=B1*2' });
  let vivo = construirMotor(m);
  const ediciones: [string, string][] = [
    ['A1', '10'],
    ['A4', '=A3*2'],
    ['A2', ''],
    ['B3', '=B2+A4'],
    ['A1', '<i>7</i>'],
    ['A3', 'texto'],
    ['A2', '5'],
  ];
  for (const [ref, html] of ediciones) {
    m = con(m, ref, html);
    vivo = sincronizarMotor(vivo, m).vivo;
    const fresco = construirMotor(m).engine;
    for (const r of ['A1', 'A2', 'A3', 'A4', 'B1', 'B2', 'B3']) {
      expect(vivo.engine.getValue(H, r)).toEqual(fresco.getValue(H, r));
    }
  }
});

test('las volátiles se reevalúan en cada edición a través del motor vivo', () => {
  const m0 = modelo({ A1: '=RAND()', B1: '1' });
  const v0 = construirMotor(m0);
  // El motor creado por construirMotor usa el reloj/azar reales: basta con ver que cambia.
  const a = v0.engine.getValue(H, 'A1');
  sincronizarMotor(v0, con(m0, 'B1', '2'));
  expect(v0.engine.getValue(H, 'A1')).not.toBe(a);
});

test('añadir una hoja reconstruye; la referencia a ella se resuelve', () => {
  const m0 = modelo({ A1: '=Datos!A1+1' });
  const v0 = construirMotor(m0);
  const m1: SheetModel = { ...m0, sheets: [...m0.sheets, { name: 'Datos', cells: { A1: '41' } }] };
  const { vivo, escritas } = sincronizarMotor(v0, m1);
  expect(escritas).toBe(-1);
  expect(vivo).not.toBe(v0);
  expect(vivo.engine.getValue(H, 'A1')).toBe(42);
});

test('Tabla[Columna]: renombrar un encabezado o redimensionar la tabla reconstruye', () => {
  const tabla = { name: 'Ventas', range: 'A1:B3' };
  const m0 = modelo(
    { A1: 'Mes', B1: 'Total', A2: 'ene', B2: '10', A3: 'feb', B3: '20', D1: '=SUM(Ventas[Total])' },
    { tables: [tabla] },
  );
  const v0 = construirMotor(m0);
  expect(v0.engine.getValue(H, 'D1')).toBe(30);

  // Una edición normal dentro de la tabla: diferencia, mismo motor.
  const m1 = con(m0, 'B2', '15');
  const s1 = sincronizarMotor(v0, m1);
  expect(s1.vivo).toBe(v0);
  expect(s1.vivo.engine.getValue(H, 'D1')).toBe(35);

  // La tabla crece una fila: el resolutor da otro rango, hay que reconstruir.
  const m2: SheetModel = {
    ...m1,
    sheets: [
      {
        ...m1.sheets[0]!,
        cells: { ...m1.sheets[0]!.cells, A4: 'mar', B4: '5' },
        tables: [{ ...tabla, range: 'A1:B4' }],
      },
    ],
  };
  const s2 = sincronizarMotor(s1.vivo, m2);
  expect(s2.escritas).toBe(-1);
  expect(s2.vivo.engine.getValue(H, 'D1')).toBe(40);

  // El estilo de la tabla NO obliga a reconstruir.
  const m3: SheetModel = {
    ...m2,
    sheets: [{ ...m2.sheets[0]!, tables: [{ ...m2.sheets[0]!.tables![0]!, style: 'otro' }] }],
  };
  expect(sincronizarMotor(s2.vivo, m3).vivo).toBe(s2.vivo);
});

test('el filtro: SUBTOTAL cuenta solo las filas visibles y sigue al editar', () => {
  // Encabezado en la fila 1; el filtro excluye «b» de la columna A.
  const af = { row: 0, c1: 0, c2: 1, excl: { '0': ['b'] } };
  const m0 = modelo(
    {
      A1: 'k',
      B1: 'v',
      A2: 'a',
      B2: '1',
      A3: 'b',
      B3: '10',
      A4: 'a',
      B4: '100',
      D1: '=SUBTOTAL(109,B2:B4)',
    },
    { autofilter: af },
  );
  const v0 = construirMotor(m0);
  expect(v0.engine.getValue(H, 'D1')).toBe(101); // la fila de «b» no cuenta

  // Editar un valor visible: se recalcula sin reconstruir.
  const m1 = con(m0, 'B2', '2');
  const s1 = sincronizarMotor(v0, m1);
  expect(s1.vivo).toBe(v0);
  expect(s1.vivo.engine.getValue(H, 'D1')).toBe(102);

  // Cambiar el texto de la fila oculta a «a»: ahora el filtro la deja ver.
  const m2 = con(m1, 'A3', 'a');
  expect(sincronizarMotor(s1.vivo, m2).vivo.engine.getValue(H, 'D1')).toBe(112);
});

test('quitar el filtro devuelve a la vista las filas que ocultaba', () => {
  const af = { row: 0, c1: 0, c2: 1, excl: { '0': ['b'] } };
  const m0 = modelo(
    { A1: 'k', B1: 'v', A2: 'a', B2: '1', A3: 'b', B3: '10', D1: '=SUBTOTAL(109,B2:B3)' },
    { autofilter: af },
  );
  const v0 = construirMotor(m0);
  expect(v0.engine.getValue(H, 'D1')).toBe(1);
  const sin: SheetData = { name: H, cells: m0.sheets[0]!.cells };
  const s = sincronizarMotor(v0, { ...m0, sheets: [sin] });
  expect(s.vivo).toBe(v0);
  expect(s.vivo.engine.getValue(H, 'D1')).toBe(11);
});

test('los nombres definidos: nacen con el motor y cambiarlos reconstruye', () => {
  const m0: SheetModel = { ...modelo({ A1: '=Tasa*100' }), names: [{ name: 'Tasa', ref: '0.21' }] };
  const v0 = construirMotor(m0);
  expect(v0.engine.getValue(H, 'A1')).toBe(21);
  const m1: SheetModel = { ...m0, names: [{ name: 'Tasa', ref: '0.1' }] };
  const { vivo, escritas } = sincronizarMotor(v0, m1);
  expect(escritas).toBe(-1);
  expect(vivo.engine.getValue(H, 'A1')).toBe(10);
});

test('con la pista de la celda editada no se compara la hoja entera', () => {
  const m0 = modelo({ A1: '1', B1: '=A1*10', C1: '=B1+1' });
  const v0 = construirMotor(m0);
  v0.engine.recalc();
  const cells = { ...m0.sheets[0]!.cells, A1: '5' };
  anotarCambio(cells, m0.sheets[0]!.cells, ['A1']);
  const { vivo, escritas } = sincronizarMotor(v0, { ...m0, sheets: [{ name: H, cells }] });
  expect(escritas).toBe(1);
  expect(vivo.engine.getValue(H, 'C1')).toBe(51);
});

test('una pista que no parte de la versión del motor se ignora y se compara entera', () => {
  const m0 = modelo({ A1: '1', B1: '=A1*10' });
  const v0 = construirMotor(m0);
  const intermedia = { ...m0.sheets[0]!.cells, A1: '2' };
  const cells = { ...intermedia, B1: '=A1*100' };
  // La pista dice «solo A1», pero parte de `intermedia`, no de lo que el motor tiene.
  anotarCambio(cells, intermedia, ['B1']);
  const { vivo } = sincronizarMotor(v0, { ...m0, sheets: [{ name: H, cells }] });
  expect(vivo.engine.getValue(H, 'B1')).toBe(200); // vio también el cambio de A1
});

test('una pista puede anotar un borrado', () => {
  const m0 = modelo({ A1: '1', B1: '=A1+1' });
  const v0 = construirMotor(m0);
  const cells = { B1: m0.sheets[0]!.cells.B1! };
  anotarCambio(cells, m0.sheets[0]!.cells, ['A1']);
  const { vivo } = sincronizarMotor(v0, { ...m0, sheets: [{ name: H, cells }] });
  expect(vivo.engine.getValue(H, 'B1')).toBe(1);
});
