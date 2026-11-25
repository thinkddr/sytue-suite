/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { MARCA_APP_LISTA, cuandoAppLista, marcarAppLista } from './appLista.js';

afterEach(() => performance.clearMarks(MARCA_APP_LISTA));

test('una sola marca `sytue:app-ready`, la primera, y avisa a quien espera', async () => {
  let listo = false;
  const espera = cuandoAppLista(undefined, 60_000).then(() => (listo = true));
  await Promise.resolve();
  expect(listo).toBe(false);
  marcarAppLista();
  marcarAppLista();
  await espera;
  expect(performance.getEntriesByName(MARCA_APP_LISTA, 'mark')).toHaveLength(1);
  // Ya marcada, no se espera.
  await cuandoAppLista(undefined, 60_000);
});

test('una app que no marca no deja esperando: el plazo resuelve', async () => {
  const antes = Date.now();
  await cuandoAppLista(undefined, 20);
  expect(Date.now() - antes).toBeGreaterThanOrEqual(15);
});

test('abortada, la espera no resuelve nunca', async () => {
  const controller = new AbortController();
  let resuelta = false;
  void cuandoAppLista(controller.signal, 10).then(() => (resuelta = true));
  controller.abort();
  await new Promise((r) => setTimeout(r, 30));
  expect(resuelta).toBe(false);
});

/**
 * Cada app que pasa por la puerta de la bóveda (`MatrixStorageGate` de
 * `@sytue/shell/vault`) pone la marca: sin ella, su bóveda espera los 5 s del plazo y
 * su presupuesto de Lighthouse no tiene nada que medir. La ofimática (Docus, Calcus,
 * Diapos, CertiPDF) monta la puerta y la marca desde `@sytue/office`.
 */
test('every app behind the vault gate marks `sytue:app-ready`', () => {
  const raiz = join(import.meta.dir, '../../../..');
  const fuentes = (dir: string): string[] =>
    readdirSync(dir, { recursive: true, encoding: 'utf8' })
      .filter((f) => /\.(ts|tsx)$/.test(f) && !/\.test\.tsx?$/.test(f) && !f.includes('dev/'))
      .map((f) => readFileSync(join(dir, f), 'utf8'));
  const raices = [
    ...readdirSync(join(raiz, 'apps')).map((app) => join(raiz, 'apps', app, 'src')),
    join(raiz, 'packages/office/src'),
  ].filter((dir) => existsSync(dir));
  const sinMarca = raices.filter((dir) => {
    const codigo = fuentes(dir);
    const conPuerta = codigo.some(
      (c) => c.includes('MatrixStorageGate') && c.includes("'@sytue/shell/vault'"),
    );
    return conPuerta && !codigo.some((c) => c.includes('marcarAppLista('));
  });
  expect(raices.length).toBeGreaterThan(10);
  expect(sinMarca.map((dir) => dir.slice(raiz.length + 1))).toEqual([]);
});
