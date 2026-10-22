/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El relleno con IA contra los vectores COMPARTIDOS con la app nativa
 * (apps/nucleo/formula/analisis/testdata/relleno.json, que Go recorre en
 * relleno_test.go): la misma hoja pide lo mismo y la misma respuesta se lee igual.
 * Con GRABAR=1 escribe lo que da la web como `esperado`.
 */
import { expect, test } from 'bun:test';
import { readFileSync, writeFileSync } from 'node:fs';
import { buildFillPrompt, parseFillResponse } from '../src/editor/sheets/relleno';

const ruta = new URL('../../../../nucleo/formula/analisis/testdata/relleno.json', import.meta.url);
interface Peticion {
  nombre: string;
  cabeceras: string[];
  filas: string[][];
  primera: number;
  objetivo: number;
  pista: string;
  esperado?: unknown;
}
interface Lectura {
  nombre: string;
  texto: string;
  pendientes: number[];
  esperado?: unknown;
}
const v = JSON.parse(readFileSync(ruta, 'utf8')) as {
  _: string;
  peticiones: Peticion[];
  lecturas: Lectura[];
};

const pedir = (c: Peticion): unknown =>
  buildFillPrompt({
    headers: c.cabeceras,
    rows: c.filas,
    firstRow: c.primera,
    target: c.objetivo,
    hint: c.pista,
  });
const leer = (c: Lectura): unknown =>
  Object.fromEntries([...parseFillResponse(c.texto, c.pendientes)].map(([k, x]) => [String(k), x]));

if (process.env['GRABAR'] === '1') {
  for (const c of v.peticiones) c.esperado = pedir(c);
  for (const c of v.lecturas) c.esperado = leer(c);
  writeFileSync(ruta, `${JSON.stringify(v).replace(/\{"nombre"/g, '\n{"nombre"')}\n`);
}

for (const c of v.peticiones) {
  test(`petición: ${c.nombre}`, () => expect(pedir(c)).toEqual(c.esperado as never));
}
for (const c of v.lecturas) {
  test(`lectura: ${c.nombre}`, () => expect(leer(c)).toEqual(c.esperado as never));
}

test('lo que importa, a mano', () => {
  const p = buildFillPrompt({
    headers: ['Nombre', 'Iniciales'],
    rows: [
      ['Ana García', 'AG'],
      ['Luis Pérez', ''],
    ],
    firstRow: 2,
    target: 1,
  });
  expect(p?.pending).toEqual([3]);
  expect(p?.context).toBe('fila\tNombre\tIniciales\n2\tAna García\tAG\n3\tLuis Pérez\t?');
  expect([...parseFillResponse('3\t-5', [3])]).toEqual([[3, '-5']]);
});
