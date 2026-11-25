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

/**
 * Y las páginas SECUNDARIAS de esas apps, que montan su propia isla: sin la marca, en
 * ellas la bóveda esperaba los 5 s (los dispositivos y los datos del portal, los álbumes
 * de Photos, el detalle de una base y el estudio de PDF de CertiPDF, medido el
 * 2026-11-24). VISTO EN ROJO con las siete sin `marcarAppLista`.
 */
test('the secondary pages behind the vault gate mark `sytue:app-ready` too', () => {
  const raiz = join(import.meta.dir, '../../../..');
  const paginas = [
    'apps/portal/src/components/ConnectDevices.tsx', // /dispositivos
    'apps/portal/src/components/TusDatos.tsx', // /datos
    'apps/photos/src/components/Albums.tsx', // /albums
    'apps/photos/src/components/AlbumView.tsx', // /album
    'apps/bases/src/islands/BaseApp.tsx', // /base/[id]
    'packages/office/src/pdf/PdfApp.tsx', // /pdf?path= de CertiPDF
    'packages/office/src/pdf/EstudioBoveda.tsx', // /pdf?path= con el Drive en la bóveda
  ];
  const sinMarca = paginas.filter(
    (f) => !readFileSync(join(raiz, f), 'utf8').includes('marcarAppLista('),
  );
  expect(sinMarca).toEqual([]);
});
