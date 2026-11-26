/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El contrato de `leerFuentes`: lo que casa sale con su texto, y lo que se queda corto
 * LANZA. Lo segundo es lo que importa; sin ello, un guardián con el patrón roto vuelve
 * a pasar en verde sin haber leído nada.
 */
import { fileURLToPath } from 'node:url';
import { expect, test } from 'bun:test';
import { leerFuentes } from './guardianes.js';

const AQUI = new URL('./', import.meta.url);
const sinTests = (rel: string): boolean => !rel.includes('.test.');

test('cada fichero sale con su texto, ordenado por ruta y después del filtro', async () => {
  const todos = await leerFuentes({ glob: 'guardianes*.ts', cwd: AQUI, minimo: 2 });
  expect(todos.map(([rel]) => rel)).toEqual(['guardianes.test.ts', 'guardianes.ts']);
  expect(todos[1]![1]).toContain('export async function leerFuentes');

  // Y el `cwd` vale también como ruta.
  const filtrados = await leerFuentes({
    glob: 'guardianes*.ts',
    cwd: fileURLToPath(AQUI),
    minimo: 1,
    filtro: sinTests,
  });
  expect(filtrados.map(([rel]) => rel)).toEqual(['guardianes.ts']);
});

test('un recorrido que se queda corto lanza y nombra el patrón', async () => {
  // Un patrón que no casa nada: es el caso que motivó todo esto.
  await expect(leerFuentes({ glob: 'no-existe/**/*.ts', cwd: AQUI, minimo: 1 })).rejects.toThrow(
    '«no-existe/**/*.ts» desde',
  );
  // El mínimo se cuenta DESPUÉS del filtro: casan dos, pasa uno y se pedían dos.
  await expect(
    leerFuentes({ glob: 'guardianes*.ts', cwd: AQUI, minimo: 2, filtro: sinTests }),
  ).rejects.toThrow('solo da 1 ficheros y tendrían que ser 2');
});
