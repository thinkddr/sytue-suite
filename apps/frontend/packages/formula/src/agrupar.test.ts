/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GROUPBY, PIVOTBY, PERCENTOF, TRIMRANGE, ISOMITTED y los LAMBDA con nombre llamados como
 * función (con parámetros opcionales). Lo que se vigila es la FORMA de lo que sale
 * (cabeceras, totales arriba o abajo, qué celda queda en blanco) además de las cifras.
 */
import { expect, test } from 'bun:test';
import { Engine } from './engine.js';
import { FormulaError } from './values.js';
import type { CellValue } from './values.js';

/** Ventas: región, producto, importe (con cabecera en la fila 1). */
const VENTAS: Record<string, string | number> = {
  A1: 'Región',
  B1: 'Producto',
  C1: 'Importe',
  A2: 'Norte',
  B2: 'Pan',
  C2: 10,
  A3: 'Sur',
  B3: 'Pan',
  C3: 20,
  A4: 'Norte',
  B4: 'Leche',
  C4: 30,
  A5: 'Sur',
  B5: 'Leche',
  C5: 40,
  A6: 'Norte',
  B6: 'Pan',
  C6: 50,
};

function motor(
  datos: Record<string, string | number>,
  nombres: Record<string, string> = {},
): Engine {
  const e = new Engine();
  for (const [n, d] of Object.entries(nombres)) expect(e.defineName(n, d)).toBe(true);
  for (const [ref, v] of Object.entries(datos)) e.setCell('Hoja1', ref, String(v));
  return e;
}

/** Evalúa en P1 y devuelve el rectángulo `alto × ancho` derramado (error → su código). */
function rect(
  formula: string,
  alto: number,
  ancho: number,
  datos: Record<string, string | number> = VENTAS,
  nombres: Record<string, string> = {},
): (CellValue | string)[][] {
  const e = motor(datos, nombres);
  e.setCell('Hoja1', 'P1', formula);
  const out: (CellValue | string)[][] = [];
  for (let r = 0; r < alto; r++) {
    const fila: (CellValue | string)[] = [];
    for (let c = 0; c < ancho; c++) {
      const v = e.getValue('Hoja1', `${String.fromCharCode(80 + c)}${r + 1}`);
      fila.push(v instanceof FormulaError ? v.code : v);
    }
    out.push(fila);
  }
  return out;
}

function uno(
  formula: string,
  datos: Record<string, string | number> = {},
  nombres: Record<string, string> = {},
): CellValue | string {
  const e = motor(datos, nombres);
  e.setCell('Hoja1', 'P1', formula);
  const v = e.getValue('Hoja1', 'P1');
  return v instanceof FormulaError ? v.code : v;
}

/* ---------- PERCENTOF ---------- */

test('PERCENTOF es la parte sobre el total, como razón', () => {
  expect(uno('=PERCENTOF(A1:A2,A1:A4)', { A1: 10, A2: 20, A3: 30, A4: 40 })).toBeCloseTo(0.3, 12);
  expect(uno('=PERCENTOF(A1,A1:A2)', { A1: 5, A2: 0 })).toBe(1);
  expect(uno('=PERCENTOF(A1,A2)', { A1: 5, A2: 0 })).toBe('#DIV/0!');
  expect(uno('=PERCENTOF(A1)', { A1: 5 })).toBe('#VALUE!');
});

/* ---------- TRIMRANGE ---------- */

test('TRIMRANGE quita lo en blanco de los bordes y no toca lo de en medio', () => {
  const datos = { B2: 1, C2: 2, B4: 3, C4: 4 }; // fila 1 y 3 en blanco; col A y D en blanco
  const m = rect('=TRIMRANGE(A1:D5)', 3, 2, datos);
  // Quedan B2:C4: la fila 3 (en blanco, en medio) se conserva.
  expect(m).toEqual([
    [1, 2],
    ['', ''],
    [3, 4],
  ]);
});

test('TRIMRANGE: 1 recorta el principio, 2 el final, 0 nada', () => {
  const datos = { B2: 1, B4: 2 };
  expect(rect('=TRIMRANGE(A1:B5,1,0)', 4, 2, datos)[0]).toEqual(['', 1]); // sin fila 1; filas 2..5, A..B
  expect(rect('=TRIMRANGE(A1:B5,2,0)', 4, 2, datos)[0]).toEqual(['', '']); // quita final: A1:B4
  expect(rect('=TRIMRANGE(A1:B5,0,3)', 5, 1, datos)[1]).toEqual([1]); // cols: solo B
  expect(uno('=TRIMRANGE(A1:B2)', {})).toBe('#CALC!'); // todo en blanco
  expect(uno('=TRIMRANGE(A1:B2,4)', { A1: 1 })).toBe('#VALUE!');
});

/* ---------- Argumentos omitidos en el parser ---------- */

test('un argumento omitido entre comas llega en blanco', () => {
  expect(uno('=SUM(1,,3)')).toBe(4);
});

/* ---------- LAMBDA con nombre, opcionales e ISOMITTED ---------- */

test('un LAMBDA definido como nombre se llama como una función', () => {
  expect(uno('=DOBLE(21)', {}, { DOBLE: 'LAMBDA(x,x*2)' })).toBe(42);
  expect(uno('=SUMA3(1,2,3)', {}, { SUMA3: 'LAMBDA(a,b,c,a+b+c)' })).toBe(6);
});

test('aridad: faltar un obligatorio o pasar de más es #VALUE!', () => {
  const n = { SUMA: 'LAMBDA(a,b,a+b)' };
  expect(uno('=SUMA(1)', {}, n)).toBe('#VALUE!');
  expect(uno('=SUMA(1,2,3)', {}, n)).toBe('#VALUE!');
});

test('parámetros opcionales con ISOMITTED', () => {
  const n = { MAS: 'LAMBDA(a,[b],IF(ISOMITTED(b),a,a+b))' };
  expect(uno('=MAS(5)', {}, n)).toBe(5);
  expect(uno('=MAS(5,2)', {}, n)).toBe(7);
  expect(uno('=MAS(5,)', {}, n)).toBe(5); // omitido explícito
});

test('un opcional antes de un obligatorio no es un LAMBDA válido', () => {
  expect(uno('=F(1,2)', {}, { F: 'LAMBDA([a],b,a+b)' })).toBe('#VALUE!');
});

test('ISOMITTED fuera de un LAMBDA, o con algo que no es un parámetro, no es un valor', () => {
  expect(uno('=ISOMITTED(x)')).toBe('#NAME?');
  expect(uno('=ISOMITTED(1)')).toBe('#VALUE!');
});

test('un LAMBDA con nombre sirve de función para MAP, y una función de la librería también', () => {
  const datos = { A1: 1, A2: 2, A3: 3 };
  expect(
    rect('=MAP(A1:A3,DOBLE)', 3, 1, datos, { DOBLE: 'LAMBDA(x,x*2)' }).map((f) => f[0]),
  ).toEqual([2, 4, 6]);
  // Eta-reducida: BYROW(rango, SUM) en vez de BYROW(rango, LAMBDA(f, SUM(f))).
  expect(rect('=BYROW(A1:B2,SUM)', 2, 1, { A1: 1, B1: 2, A2: 3, B2: 4 }).map((f) => f[0])).toEqual([
    3, 7,
  ]);
});

test('un LAMBDA con nombre puede llamarse a sí mismo y hay un tope de profundidad', () => {
  const fact = { FACT2: 'LAMBDA(n,IF(n<=1,1,n*FACT2(n-1)))' };
  expect(uno('=FACT2(5)', {}, fact)).toBe(120);
  expect(uno('=INF(1)', {}, { INF: 'LAMBDA(n,INF(n+1))' })).toBe('#NUM!');
});

test('editar lo que lee el cuerpo de un LAMBDA con nombre recalcula a quien lo llama', () => {
  const e = motor({ A1: 10 }, { SUMAA1: 'LAMBDA(x,x+A1)' });
  e.setCell('Hoja1', 'B1', '=SUMAA1(1)');
  expect(e.getValue('Hoja1', 'B1')).toBe(11);
  e.setCell('Hoja1', 'A1', '20');
  expect(e.getValue('Hoja1', 'B1')).toBe(21);
});

test('un LAMBDA con nombre cuyo cuerpo es volátil se reevalúa en cada edición', () => {
  let n = 0;
  const e = new Engine({ random: () => ++n / 100 });
  e.defineName('AZAR', 'LAMBDA(x,x+RAND())');
  e.setCell('Hoja1', 'B1', '=AZAR(1)');
  const a = e.getValue('Hoja1', 'B1');
  e.setCell('Hoja1', 'H9', 'nota');
  expect(e.getValue('Hoja1', 'B1')).not.toBe(a);
});

/* ---------- GROUPBY ---------- */

test('GROUPBY suma por grupo, con cabecera detectada y total general abajo', () => {
  const m = rect('=GROUPBY(A1:A6,C1:C6,SUM)', 4, 2);
  // Las cabeceras se detectan (texto sobre número) y se muestran.
  expect(m).toEqual([
    ['Región', 'Importe'],
    ['Norte', 90],
    ['Sur', 60],
    ['Total', 150],
  ]);
});

test('GROUPBY: sin cabeceras, sin total y orden descendente por el valor', () => {
  const datos = { A1: 'a', A2: 'b', A3: 'a', B1: 1, B2: 5, B3: 2 };
  const m = rect('=GROUPBY(A1:A3,B1:B3,SUM,0,0,-2)', 2, 2, datos);
  expect(m).toEqual([
    ['b', 5],
    ['a', 3],
  ]);
});

test('GROUPBY: total arriba si el total es negativo', () => {
  const m = rect('=GROUPBY(A2:A6,C2:C6,SUM,0,-1)', 3, 2);
  expect(m[0]).toEqual(['Total', 150]);
});

test('GROUPBY con dos campos repite las etiquetas y ordena por la clave', () => {
  const m = rect('=GROUPBY(A2:B6,C2:C6,SUM,0,0)', 4, 3);
  expect(m).toEqual([
    ['Norte', 'Leche', 30],
    ['Norte', 'Pan', 60],
    ['Sur', 'Leche', 40],
    ['Sur', 'Pan', 20],
  ]);
});

test('GROUPBY con subtotales (total 2): un «Total» tras cada grupo exterior y el general', () => {
  const m = rect('=GROUPBY(A2:B6,C2:C6,SUM,0,2)', 7, 3);
  expect(m).toEqual([
    ['Norte', 'Leche', 30],
    ['Norte', 'Pan', 60],
    ['Norte', 'Total', 90],
    ['Sur', 'Leche', 40],
    ['Sur', 'Pan', 20],
    ['Sur', 'Total', 60],
    ['Total', '', 150],
  ]);
});

test('GROUPBY con PERCENTOF da la parte de cada grupo sobre el total', () => {
  const m = rect('=GROUPBY(A2:A6,C2:C6,PERCENTOF,0,0)', 2, 2);
  expect(m[0]![0]).toBe('Norte');
  expect(m[0]![1] as number).toBeCloseTo(0.6, 12);
  expect(m[1]![1] as number).toBeCloseTo(0.4, 12);
});

test('GROUPBY con un LAMBDA como función y un filtro', () => {
  // Máximo por región solo de lo que no es «Leche».
  const m = rect('=GROUPBY(A2:A6,C2:C6,LAMBDA(g,MAX(g)),0,0,,B2:B6<>"Leche")', 2, 2);
  expect(m).toEqual([
    ['Norte', 50],
    ['Sur', 20],
  ]);
});

test('GROUPBY con varias columnas de valores resume cada una', () => {
  const datos = { A1: 'x', A2: 'x', A3: 'y', B1: 1, B2: 2, B3: 3, C1: 10, C2: 20, C3: 30 };
  expect(rect('=GROUPBY(A1:A3,B1:C3,SUM,0,0)', 2, 3, datos)).toEqual([
    ['x', 3, 30],
    ['y', 3, 30],
  ]);
});

test('GROUPBY: dimensiones que no cuadran, opciones inválidas y filtro que lo deja todo fuera', () => {
  expect(uno('=GROUPBY(A2:A6,C2:C5,SUM)', VENTAS)).toBe('#VALUE!');
  expect(uno('=GROUPBY(A2:A6,C2:C6,SUM,0,9)', VENTAS)).toBe('#VALUE!');
  expect(uno('=GROUPBY(A2:A6,C2:C6,SUM,0,1,7)', VENTAS)).toBe('#VALUE!'); // orden fuera de rango
  expect(uno('=GROUPBY(A2:A6,C2:C6,SUM,0,1,,C2:C6>999)', VENTAS)).toBe('#CALC!');
  expect(uno('=GROUPBY(A2:A6,C2:C6)', VENTAS)).toBe('#VALUE!'); // falta la función
});

test('GROUPBY: un error en el agregador sale como el valor de esa celda', () => {
  const m = rect('=GROUPBY(A2:A3,C2:C3,LAMBDA(g,1/0),0,0)', 2, 2);
  expect(m).toEqual([
    ['Norte', '#DIV/0!'],
    ['Sur', '#DIV/0!'],
  ]);
});

/* ---------- PIVOTBY ---------- */

test('PIVOTBY cruza filas y columnas con totales a la derecha y abajo', () => {
  const m = rect('=PIVOTBY(A1:A6,B1:B6,C1:C6,SUM)', 4, 4);
  expect(m).toEqual([
    ['Región', 'Leche', 'Pan', 'Total'],
    ['Norte', 30, 60, 90],
    ['Sur', 40, 20, 60],
    ['Total', 70, 80, 150],
  ]);
});

test('PIVOTBY deja en blanco el cruce sin datos y admite quitar totales', () => {
  const datos = { A1: 'a', A2: 'b', B1: 'x', B2: 'y', C1: 1, C2: 2 };
  const m = rect('=PIVOTBY(A1:A2,B1:B2,C1:C2,SUM,0,0,,0)', 3, 3, datos);
  expect(m).toEqual([
    ['', 'x', 'y'],
    ['a', 1, ''],
    ['b', '', 2],
  ]);
});

test('PIVOTBY con totales negativos los pone arriba y a la izquierda', () => {
  const m = rect('=PIVOTBY(A2:A6,B2:B6,C2:C6,SUM,0,-1,,-1)', 4, 4);
  expect(m[0]![0]).toBe('');
  expect(m[0]![1]).toBe('Total');
  expect(m[1]).toEqual(['Total', 150, 70, 80]);
});

test('PIVOTBY: varias columnas de valores y subtotales de varios campos no se admiten', () => {
  expect(uno('=PIVOTBY(A2:A6,B2:B6,C2:D6,SUM)', VENTAS)).toBe('#VALUE!');
  expect(uno('=PIVOTBY(A2:B6,B2:B6,C2:C6,SUM,0,2)', VENTAS)).toBe('#VALUE!');
});
