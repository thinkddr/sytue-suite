// Checks del reescrito de referencias al insertar/borrar filas y columnas.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  moverCriterios,
  moverObjetos,
  moverReferencia,
  moverReferenciasAjenas,
  moverVista,
  reescribirFormula,
  REF_ROTA,
  type Desplazamiento,
} from '../src/editor/sheets/refshift';
import type { SheetData } from '../src/editor/types';

/** Insertar `n` filas en `desde` (0-based). Rejilla de 100×26, como el lienzo. */
const insFila = (desde: number, n = 1): Desplazamiento => ({
  eje: 'fila',
  desde,
  cantidad: n,
  tope: 100,
});
const delFila = (desde: number, n = 1): Desplazamiento => ({
  eje: 'fila',
  desde,
  cantidad: -n,
  tope: 100,
});
const insCol = (desde: number, n = 1): Desplazamiento => ({
  eje: 'columna',
  desde,
  cantidad: n,
  tope: 26,
});
const delCol = (desde: number, n = 1): Desplazamiento => ({
  eje: 'columna',
  desde,
  cantidad: -n,
  tope: 26,
});

test('lo de antes del corte no se mueve; lo de después, sí', () => {
  // Insertando en la fila 3 (índice 2): A1 y A2 se quedan, A3 baja a A4.
  expect(moverReferencia('A1', insFila(2))).toBe('A1');
  expect(moverReferencia('A2', insFila(2))).toBe('A2');
  expect(moverReferencia('A3', insFila(2))).toBe('A4');
  expect(moverReferencia('A10', insFila(2))).toBe('A11');
});

test('un rango al que le insertan dentro se ESTIRA', () => {
  // ES EL BUG QUE ESTO ARREGLA: `=SUMA(A1:A5)` seguía sumando cinco filas cuando
  // ya eran seis. Ni error ni aviso: solo un resultado que pasa a estar mal.
  expect(reescribirFormula('=SUMA(A1:A5)', insFila(2))).toBe('=SUMA(A1:A6)');
  // Insertar POR ENCIMA del rango lo baja entero, no lo estira.
  expect(reescribirFormula('=SUMA(A3:A5)', insFila(0))).toBe('=SUMA(A4:A6)');
  // Insertar POR DEBAJO no lo toca.
  expect(reescribirFormula('=SUMA(A1:A5)', insFila(20))).toBe('=SUMA(A1:A5)');
});

test('borrar lo que una fórmula señalaba da #¡REF!, no un cero', () => {
  // Un cero silencioso es peor que un error: se suma, se firma y se manda.
  expect(moverReferencia('A3', delFila(2))).toBe(REF_ROTA);
  expect(reescribirFormula('=A3*2', delFila(2))).toBe(`=${REF_ROTA}*2`);
  // Lo de después se sube.
  expect(moverReferencia('A9', delFila(2, 3))).toBe('A6');
});

test('un rango se ENCOGE si le borran un trozo, y muere si se lo borran entero', () => {
  expect(reescribirFormula('=SUMA(A1:A10)', delFila(2, 3))).toBe('=SUMA(A1:A7)');
  // Borrado 3-5 (índices 2-4) y rango A4:A8 → el inicio se pega al borde.
  expect(reescribirFormula('=SUMA(A4:A8)', delFila(2, 3))).toBe('=SUMA(A3:A5)');
  // El rango entero desaparece.
  expect(reescribirFormula('=SUMA(A3:A5)', delFila(2, 3))).toBe(`=SUMA(${REF_ROTA})`);
  // Si lo borrado se lleva el FINAL, el fin se pega a la última que queda antes del
  // corte: `A1:A3` ya sería la antigua fila 6, un dato que el rango nunca tuvo.
  expect(reescribirFormula('=SUMA(A1:A5)', delFila(2, 3))).toBe('=SUMA(A1:A2)');
  expect(reescribirFormula('=SUMA(A5:A1)', delFila(2, 3))).toBe('=SUMA(A2:A1)');
  expect(moverReferencia('B2:E2', delCol(3, 5))).toBe('B2:C2');
});

test('los $ NO eximen del desplazamiento', () => {
  // Es el error clásico: `$` marca la referencia como absoluta al COPIAR, que es
  // otra cosa. Con una fila insertada encima, el dato se ha movido DE VERDAD.
  expect(moverReferencia('$A$3', insFila(0))).toBe('$A$4');
  expect(moverReferencia('A$3', insFila(0))).toBe('A$4');
  expect(reescribirFormula('=$B$2+$B$7', insFila(3))).toBe('=$B$2+$B$8');
});

test('las columnas se mueven igual que las filas', () => {
  expect(moverReferencia('C1', insCol(1))).toBe('D1');
  expect(moverReferencia('A1', insCol(1))).toBe('A1');
  expect(reescribirFormula('=SUMA(B1:D1)', insCol(2))).toBe('=SUMA(B1:E1)');
  expect(moverReferencia('C1', delCol(2))).toBe(REF_ROTA);
  expect(moverReferencia('E1', delCol(2, 2))).toBe('C1');
});

test('una referencia a la columna entera no la mueve una fila', () => {
  // `A:A` es la columna A haya las filas que haya; tocarla al insertar una fila
  // sería inventarse un cambio.
  expect(moverReferencia('A:A', insFila(3))).toBe('A:A');
  expect(moverReferencia('3:3', insCol(1))).toBe('3:3');
  // Pero una columna entera SÍ se mueve al insertar columnas.
  expect(moverReferencia('B:B', insCol(0))).toBe('C:C');
});

test('la hoja y las mayúsculas del original se respetan', () => {
  expect(moverReferencia('Hoja1!B4', insFila(0))).toBe('Hoja1!B5');
  expect(moverReferencia("'Mi hoja'!B4:C9", insFila(0))).toBe("'Mi hoja'!B5:C10");
  // Quien escribió en minúsculas no ha pedido que se le reescriba el texto.
  expect(moverReferencia('b4', insFila(0))).toBe('b5');
});

test('lo que se sale de la rejilla deja de existir', () => {
  // La rejilla es fija (100 filas): empujar la última fuera no puede dejar la
  // referencia pegada al borde apuntando a un dato que no es el suyo.
  expect(moverReferencia('A100', insFila(0))).toBe(REF_ROTA);
  expect(moverReferencia('Z1', insCol(0))).toBe(REF_ROTA);
});

test('lo que no es una referencia se queda como está', () => {
  expect(reescribirFormula('123', insFila(0))).toBe('123');
  expect(reescribirFormula('texto suelto', insFila(0))).toBe('texto suelto');
  expect(reescribirFormula('=SUMA(1;2)', insFila(0))).toBe('=SUMA(1;2)');
  // Y el texto de dentro de una cadena tampoco: no es una referencia.
  expect(reescribirFormula('="A3 es el total"', insFila(0))).toBe('="A3 es el total"');
});

test('varias referencias en la misma fórmula se reescriben todas', () => {
  // Se sustituye de derecha a izquierda; hacerlo al revés desplaza el texto bajo
  // los pies de las sustituciones siguientes y sale una fórmula corrupta.
  expect(reescribirFormula('=A5+B5+C5', insFila(0))).toBe('=A6+B6+C6');
  expect(reescribirFormula('=SI(A5>0;SUMA(B5:B9);C5)', insFila(0))).toBe(
    '=SI(A6>0;SUMA(B6:B10);C6)',
  );
});

// El autofiltro y las vistas de filtro (las de cada persona, fuera del modelo) siguen a las
// columnas y a las filas al insertar y borrar; borrar una columna se lleva sus criterios.
test('el autofiltro y las vistas de filtro siguen la inserción y el borrado', () => {
  const criterios = {
    excl: { '0': ['a'], '2': ['b'], '3': ['c'] },
    cond: { '1': { op: 'gt' as const, a: '5' }, '3': { op: 'eq' as const, a: 'x' } },
  };
  const hoja = {
    name: 'H',
    cells: {},
    autofilter: { row: 1, c1: 0, c2: 3, ...criterios },
  };
  const vista = { id: 'v1', name: 'Mi vista', ...criterios };

  // Insertar una columna en B (índice 1): lo de B en adelante se desplaza una.
  const ins = insCol(1);
  expect(moverObjetos(hoja, ins).autofilter).toEqual({
    row: 1,
    c1: 0,
    c2: 4,
    excl: { '0': ['a'], '3': ['b'], '4': ['c'] },
    cond: { '2': { op: 'gt', a: '5' }, '4': { op: 'eq', a: 'x' } },
  });
  expect(moverVista(vista, ins)).toEqual({
    id: 'v1',
    name: 'Mi vista',
    excl: { '0': ['a'], '3': ['b'], '4': ['c'] },
    cond: { '2': { op: 'gt', a: '5' }, '4': { op: 'eq', a: 'x' } },
  });

  // Borrar la columna C (índice 2): se va su exclusión y lo de después retrocede.
  const del = delCol(2);
  expect(moverObjetos(hoja, del).autofilter).toEqual({
    row: 1,
    c1: 0,
    c2: 2,
    excl: { '0': ['a'], '2': ['c'] },
    cond: { '1': { op: 'gt', a: '5' }, '2': { op: 'eq', a: 'x' } },
  });
  expect(moverVista(vista, del).excl).toEqual({ '0': ['a'], '2': ['c'] });

  // Una fila no toca columnas: los criterios quedan; el autofiltro baja con su fila de cabecera.
  expect(moverCriterios(criterios, insFila(0))).toEqual(criterios);
  expect(moverObjetos(hoja, insFila(0)).autofilter?.row).toBe(2);

  // Sin criterios no sobra ninguna clave vacía.
  expect(moverVista({ id: 'v2', name: 'Vacía' }, del)).toEqual({ id: 'v2', name: 'Vacía' });
});

// El formato condicional y las validaciones siguen a sus celdas (antes se quedaban en las
// de antes y pintaban o vetaban filas ajenas sin ningún aviso).
test('el formato condicional y las validaciones siguen a sus celdas', () => {
  const hoja = {
    name: 'H',
    cells: {},
    condFormats: [
      { range: 'B2:B10', kind: 'formula' as const, formula: '=B2>$D$1', fill: '#FFEB9C' },
      { range: 'C3', op: 'gt' as const, value: '5', fill: '#FFC7CE' },
    ],
    validations: [{ range: 'B2:B10', type: 'formula' as const, formula: '=B2<>""' }],
  };
  // Insertar una fila encima: la regla B2:B10 pasa a B3:B11 y su fórmula, con ella.
  const ins = moverObjetos(hoja, insFila(0));
  expect(ins.condFormats).toEqual([
    { range: 'B3:B11', kind: 'formula', formula: '=B3>$D$2', fill: '#FFEB9C' },
    { range: 'C4', op: 'gt', value: '5', fill: '#FFC7CE' },
  ]);
  expect(ins.validations).toEqual([{ range: 'B3:B11', type: 'formula', formula: '=B3<>""' }]);
  // Insertar dentro: la regla crece y su fórmula, que mira a la primera celda, no cambia.
  expect(moverObjetos(hoja, insFila(4)).condFormats?.[0]?.range).toBe('B2:B11');
  // Borrar su primera fila: la fórmula pasa a ser la de la nueva primera celda, no #¡REF!.
  const sinPrimera = moverObjetos(hoja, delFila(1));
  expect(sinPrimera.condFormats?.[0]).toMatchObject({ range: 'B2:B9', formula: '=B2>$D$1' });
  expect(sinPrimera.validations?.[0]).toMatchObject({ range: 'B2:B9', formula: '=B2<>""' });
  // Borrar todas sus filas la elimina; la que queda en una celda es esa celda, no «B2:B2».
  const todas = moverObjetos(hoja, delFila(1, 9));
  expect(todas.condFormats).toBeUndefined();
  expect(todas.validations).toBeUndefined();
  expect(moverObjetos(hoja, delFila(2, 8)).validations?.[0]?.range).toBe('B2');
  // Una columna: la regla de C se va con C; la de B no se entera.
  const sinC = moverObjetos(hoja, delCol(2));
  expect(sinC.condFormats?.map((c) => c.range)).toEqual(['B2:B10']);
  expect(sinC.condFormats?.[0]?.formula).toBe('=B2>$C$1');
});

// Con la hoja protegida, lo que el servidor veta sin su permiso no se toca: si se movieran,
// rechazaría el guardado entero (services/docs/internal/domain/proteccion.go).
test('con la hoja protegida sin «objects», el FC y las validaciones se quedan quietos', () => {
  const hoja = {
    name: 'H',
    cells: {},
    protect: { insertRows: true },
    condFormats: [{ range: 'B2:B10', op: 'gt' as const, value: '5', fill: '#FFC7CE' }],
    validations: [{ range: 'B2:B10', values: ['Sí', 'No'] }],
    charts: [{ id: 'g', type: 'bar' as const, range: 'B2:C10' }],
    autofilter: { row: 1, c1: 1, c2: 2 },
    tables: [{ name: 'T', range: 'B2:C10' }],
  };
  const out = moverObjetos(hoja, insFila(0));
  expect(out.condFormats).toBe(hoja.condFormats);
  expect(out.validations).toBe(hoja.validations);
  expect(out.charts).toBe(hoja.charts);
  expect(out.autofilter).toBe(hoja.autofilter);
  // Las tablas no las gobierna ningún permiso: se mueven.
  expect(out.tables?.[0]?.range).toBe('B3:C11');
  const concedido = moverObjetos(
    { ...hoja, protect: { insertRows: true, objects: true } },
    insFila(0),
  );
  expect(concedido.condFormats?.[0]?.range).toBe('B3:B11');
  expect(concedido.autofilter).toBe(hoja.autofilter);
});

// Los rangos editables viajan con sus celdas (D-CALC-02): si no, el de Ana cubriría el
// contenido bloqueado que baja a su primera fila. El servidor reproduce el mismo
// desplazamiento (services/docs/internal/domain/proteccion_desplazar.go).
test('los rangos editables de la hoja protegida siguen a sus celdas', () => {
  const hoja = {
    name: 'H',
    cells: {},
    protect: {
      hasPassword: true,
      insertRows: true,
      ranges: [
        { ref: 'B2:B3', users: ['ana'] },
        { ref: 'A1:A2', groups: ['contabilidad'] },
        { ref: 'D5', users: ['luis'] },
      ],
    },
  };
  const arriba = moverObjetos(hoja, insFila(0));
  expect(arriba.protect).toEqual({
    hasPassword: true,
    insertRows: true,
    ranges: [
      { ref: 'B3:B4', users: ['ana'] },
      { ref: 'A2:A3', groups: ['contabilidad'] },
      { ref: 'D6', users: ['luis'] },
    ],
  });
  // Dentro de un rango, crece; por debajo de todos, nada cambia.
  expect(moverObjetos(hoja, insFila(2)).protect?.ranges?.map((g) => g.ref)).toEqual([
    'B2:B4',
    'A1:A2',
    'D6',
  ]);
  expect(moverObjetos(hoja, insFila(10)).protect?.ranges).toEqual(hoja.protect.ranges);
  // Borrar encoge, y lo que se borra entero desaparece.
  expect(moverObjetos(hoja, delFila(1, 2)).protect?.ranges?.map((g) => g.ref)).toEqual([
    'A1',
    'D3',
  ]);
  expect(moverObjetos(hoja, insCol(0, 2)).protect?.ranges?.map((g) => g.ref)).toEqual([
    'D2:D3',
    'C1:C2',
    'F5',
  ]);
  // Sin ningún rango vivo, la hoja SIGUE protegida: solo se va la lista.
  const sinRangos = moverObjetos(
    { ...hoja, protect: { insertRows: true, ranges: [{ ref: 'B2', users: ['ana'] }] } },
    delFila(1),
  );
  expect(sinRangos.protect).toEqual({ insertRows: true });
  // Sin rangos, la protección es la misma de antes.
  const sinLista = { name: 'H', cells: {}, protect: { insertRows: true } };
  expect(moverObjetos(sinLista, insFila(0)).protect).toBe(sinLista.protect);
});

// El esquema crece, encoge o desaparece con su detalle; los anchos, los altos y las
// combinadas, igual.
test('el esquema, los anchos y las combinadas siguen a sus filas y columnas', () => {
  const hoja = {
    name: 'H',
    cells: {},
    rowGroups: [
      { start: 1, end: 4, level: 1 },
      { start: 2, end: 3, level: 2, collapsed: true },
    ],
    colGroups: [{ start: 1, end: 2, level: 1 }],
    merges: ['A1:C1', 'B5:B6'],
    colWidths: { '1': 120, '3': 60 },
    rowHeights: { '4': 30 },
  };
  // Encima: baja entero. Dentro: crece. En el resumen (end+1): no lo estira.
  expect(moverObjetos(hoja, insFila(1)).rowGroups).toEqual([
    { start: 2, end: 5, level: 1 },
    { start: 3, end: 4, level: 2, collapsed: true },
  ]);
  expect(moverObjetos(hoja, insFila(3)).rowGroups).toEqual([
    { start: 1, end: 5, level: 1 },
    { start: 2, end: 4, level: 2, collapsed: true },
  ]);
  expect(moverObjetos(hoja, insFila(5)).rowGroups).toEqual(hoja.rowGroups);
  // Borrar el detalle interior entero se lleva el grupo interior y encoge el exterior.
  expect(moverObjetos(hoja, delFila(2, 2)).rowGroups).toEqual([{ start: 1, end: 2, level: 1 }]);
  // Las filas no tocan el esquema de columnas, y al revés.
  expect(moverObjetos(hoja, insFila(0)).colGroups).toBe(hoja.colGroups);
  const conCol = moverObjetos(hoja, insCol(0));
  expect(conCol.colGroups).toEqual([{ start: 2, end: 3, level: 1 }]);
  expect(conCol.rowGroups).toBe(hoja.rowGroups);
  expect(conCol.colWidths).toEqual({ '2': 120, '4': 60 });
  expect(conCol.merges).toEqual(['B1:D1', 'C5:C6']);
  expect(moverObjetos(hoja, delCol(1, 2)).colGroups).toBeUndefined();
  const sinFila = moverObjetos(hoja, delFila(4));
  expect(sinFila.merges).toEqual(['A1:C1']);
  expect(sinFila.rowHeights).toBeUndefined();
});

// Las fórmulas de OTRAS hojas que citan la que se mueve. Antes solo se reescribía la
// hoja activa: «Resumen» con `=Ventas!B4` seguía mirando B4 después de insertar una
// fila encima en «Ventas», y la hoja activa movía también sus `=Resumen!A5`, que no
// tenían nada que ver.

const enVentas = (d: Desplazamiento): Desplazamiento => ({ ...d, hoja: 'Ventas' });

test('en la hoja que se mueve, solo sus referencias: las de otra hoja se quedan', () => {
  const d = enVentas(insFila(1));
  expect(reescribirFormula('=B4+Resumen!B4+Ventas!$B$4', d)).toBe('=B5+Resumen!B4+Ventas!$B$5');
  // Sin `hoja` (quien no sabe de libros) se mueven todas, como siempre.
  expect(reescribirFormula('=B4+Resumen!B4', insFila(1))).toBe('=B5+Resumen!B5');
});

test('desde otra hoja: celdas, rangos, comillas, $ y mayúsculas', () => {
  const d = enVentas(insFila(1));
  const f = (t: string): string => reescribirFormula(t, d, 'Resumen');
  expect(f('=Ventas!B4')).toBe('=Ventas!B5');
  expect(f('=SUM(Ventas!A1:B9)')).toBe('=SUM(Ventas!A1:B10)');
  expect(f('=ventas!$B$4*B4')).toBe('=ventas!$B$5*B4'); // la suya, B4, no
  expect(f('=Ventas!A1')).toBe('=Ventas!A1'); // antes del corte
  const conEspacio: Desplazamiento = { ...insCol(1), hoja: 'Hoja 2' };
  expect(reescribirFormula("='Hoja 2'!C3+Hoja2!C3", conEspacio, 'Resumen')).toBe(
    "='Hoja 2'!D3+Hoja2!C3",
  );
});

test('desde otra hoja, borrar la fila referida da #¡REF! y un rango se encoge', () => {
  const d = enVentas(delFila(3)); // borra la fila 4
  const f = (t: string): string => reescribirFormula(t, d, 'Resumen');
  expect(f('=Ventas!B4')).toBe(`=${REF_ROTA}`);
  expect(f('=SUM(Ventas!B2:B6)')).toBe('=SUM(Ventas!B2:B5)');
  expect(f('=Ventas!B9')).toBe('=Ventas!B8');
});

test('moverReferenciasAjenas: celdas, formato condicional y validaciones de la otra hoja', () => {
  const resumen: SheetData = {
    name: 'Resumen',
    cells: { A1: '=Ventas!B4', A2: '<b>=Ventas!B4*2</b>', A3: '=B4', A4: 'Ventas!B4' },
    condFormats: [{ range: 'A1:A3', kind: 'formula', formula: '=A1>Ventas!$B$4' }],
    validations: [{ range: 'C1', type: 'formula', formula: '=COUNTIF(Ventas!$A$1:$A$9,C1)>0' }],
  };
  const m = moverReferenciasAjenas(resumen, enVentas(insFila(0)));
  expect(m.cells).toEqual({
    A1: '=Ventas!B5',
    A2: '<b>=Ventas!B5*2</b>',
    A3: '=B4',
    A4: 'Ventas!B4',
  });
  expect(m.condFormats?.[0]?.formula).toBe('=A1>Ventas!$B$5');
  expect(m.condFormats?.[0]?.range).toBe('A1:A3');
  expect(m.validations?.[0]?.formula).toBe('=COUNTIF(Ventas!$A$2:$A$10,C1)>0');
  // La propia hoja y una hoja que no la cita salen tal cual (el mismo objeto).
  const ventas = { name: 'Ventas', cells: { A1: '=B4' } } as SheetData;
  expect(moverReferenciasAjenas(ventas, enVentas(insFila(0)))).toBe(ventas);
  const otra = { name: 'Otra', cells: { A1: '=B4' } } as SheetData;
  expect(moverReferenciasAjenas(otra, enVentas(insFila(0)))).toBe(otra);
});

// Seguir a su dato no es editar la fórmula: con la hoja protegida se reescriben también
// las celdas bloqueadas (el servidor acepta exactamente este desplazamiento,
// proteccion_ajenas.go); las reglas siguen necesitando `objects`.
test('moverReferenciasAjenas en una hoja protegida mueve también las celdas bloqueadas', () => {
  const s = {
    name: 'Resumen',
    cells: { A1: '=Ventas!B4', A2: '=Ventas!B4' },
    styles: { A2: { locked: false } },
    protect: {},
    validations: [{ range: 'C1', type: 'formula', formula: '=COUNTIF(Ventas!$A$1:$A$9,C1)>0' }],
  } as SheetData;
  const m = moverReferenciasAjenas(s, enVentas(insFila(0)));
  expect(m.cells).toEqual({ A1: '=Ventas!B5', A2: '=Ventas!B5' });
  expect(m.validations?.[0]?.formula).toBe('=COUNTIF(Ventas!$A$1:$A$9,C1)>0');
});

// Las tablas dinámicas siguen a sus datos: el origen con la hoja de la que leen y la
// esquina con la suya. Antes se quedaban quietos y leían una fila de más o de menos.
const dinamica = (extra: Partial<SheetPivotT> = {}): SheetPivotT => ({
  name: 'TD1',
  source: 'A1:C9',
  anchor: 'H2',
  range: 'H2:I5',
  rows: [{ field: 0 }],
  cols: [],
  values: [{ field: 2, fn: 'sum' }],
  ...extra,
});
type SheetPivotT = NonNullable<SheetData['pivots']>[number];

test('moverObjetos mueve el origen, la esquina y lo que ocupa una tabla dinámica', () => {
  const s = { name: 'Ventas', cells: {}, pivots: [dinamica()] } as SheetData;
  const enVentasD = (d: Desplazamiento): Desplazamiento => ({ ...d, hoja: 'Ventas' });
  expect(moverObjetos(s, enVentasD(insFila(0))).pivots).toEqual([
    dinamica({ source: 'A2:C10', anchor: 'H3', range: 'H3:I6' }),
  ]);
  // Dentro del origen: crece; la esquina, por encima, se queda.
  expect(moverObjetos(s, enVentasD(insFila(4, 2))).pivots).toEqual([
    dinamica({ source: 'A1:C11', range: 'H2:I7' }),
  ]);
  expect(moverObjetos(s, enVentasD(insCol(1))).pivots).toEqual([
    dinamica({ source: 'A1:D9', anchor: 'I2', range: 'I2:J5' }),
  ]);
  // Borrar la fila de su esquina: pasa a la primera que queda de lo que ocupaba.
  expect(moverObjetos(s, enVentasD(delFila(1))).pivots).toEqual([
    dinamica({ source: 'A1:C8', anchor: 'H2', range: 'H2:I4' }),
  ]);
  // Sin origen, la tabla dinámica desaparece como un gráfico sin datos.
  expect(moverObjetos(s, enVentasD(delCol(0, 3))).pivots).toBeUndefined();
  // Una que lee de otra hoja solo mueve su esquina.
  const otra = { name: 'Ventas', cells: {}, pivots: [dinamica({ sourceSheet: 'Datos' })] };
  expect(moverObjetos(otra as SheetData, enVentasD(insFila(0))).pivots).toEqual([
    dinamica({ sourceSheet: 'Datos', anchor: 'H3', range: 'H3:I6' }),
  ]);
  // Con la hoja protegida no se toca: el servidor veta cambiar `pivots`.
  const prot = { ...s, protect: { objects: true } } as SheetData;
  expect(moverObjetos(prot, enVentasD(insFila(0))).pivots).toEqual([dinamica()]);
});

test('moverReferenciasAjenas mueve el origen de las tablas dinámicas que leen de la hoja', () => {
  const s = {
    name: 'Resumen',
    cells: {},
    pivots: [dinamica({ sourceSheet: 'Ventas' }), dinamica({ name: 'TD2' })],
  } as SheetData;
  const m = moverReferenciasAjenas(s, enVentas(insFila(0)));
  // La de «Ventas» mueve su origen, no su esquina; la que lee de «Resumen», nada.
  expect(m.pivots).toEqual([
    dinamica({ sourceSheet: 'Ventas', source: 'A2:C10' }),
    dinamica({ name: 'TD2' }),
  ]);
  expect(moverReferenciasAjenas(s, enVentas(insFila(20)))).toBe(s);
  // Si se borra su origen entero, desaparece.
  const sola = { ...s, pivots: [dinamica({ sourceSheet: 'Ventas' })] } as SheetData;
  expect(moverReferenciasAjenas(sola, enVentas(delFila(0, 9))).pivots).toBeUndefined();
});
