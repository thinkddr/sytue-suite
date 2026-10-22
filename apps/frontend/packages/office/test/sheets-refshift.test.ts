// Checks del reescrito de referencias al insertar/borrar filas y columnas.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  moverCriterios,
  moverObjetos,
  moverReferencia,
  moverVista,
  reescribirFormula,
  REF_ROTA,
  type Desplazamiento,
} from '../src/editor/sheets/refshift';

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
