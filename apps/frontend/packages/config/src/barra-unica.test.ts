/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UNA SOLA BARRA EN EL TELÉFONO, EN TODAS LAS APPS.
 *
 * Por debajo de 768 la topbar de la suite acoge la barra de cada app (`BarraMovil` de
 * @sytue/ui, docs/design.md §2). Hasta el 2026-11 cada armazón tenía la suya y el
 * teléfono apilaba dos: AppShell y AdminShell con su barra de 44, la cabecera del canal
 * de Rooms y el ☰ suelto del ERP. Este test para a quien vuelva a escribir una a mano:
 *
 *   · la barra de 44 (`sytue-mobilebar`) solo la pinta `BarraMovil`, y solo sin topbar
 *     que la acoja (la app nativa);
 *   · un mando solo-móvil (`md:hidden`) que abre un cajón (`aria-controls`) en una app
 *     que monta la Sytue Shell tiene que pasar por `BarraMovil` o quedar en
 *     `SinBarraUnica` / `useBarraUnica`: si no, es una segunda barra (o un ☰ suelto
 *     bajo la barra) en el teléfono.
 *
 * Las capturas de las diecinueve superficies a 390 están en el resultado de la mejora.
 *
 * Visto en ROJO: devolviendo el ☰ del ERP a fuera de `SinBarraUnica` (falla el segundo)
 * y pegando `sytue-mobilebar` en Rooms (falla el primero).
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../', import.meta.url);
const FUENTES = ['apps/*/src/**/*.tsx', 'packages/*/src/**/*.tsx'];
const esTest = (rel: string): boolean => /\.test\.tsx$|\/dev\/|\/test\//.test(rel);

async function* fuentes(): AsyncGenerator<[string, string]> {
  for (const patron of FUENTES) {
    for await (const rel of new Bun.Glob(patron).scan({ cwd: RAIZ.pathname })) {
      if (esTest(rel)) continue;
      yield [rel, await Bun.file(new URL(rel, RAIZ)).text()];
    }
  }
}

test('la barra de 44 solo la pinta BarraMovil', async () => {
  const pintan: string[] = [];
  for await (const [rel, src] of fuentes()) {
    if (src.includes("'sytue-mobilebar") || src.includes('"sytue-mobilebar')) pintan.push(rel);
  }
  expect(pintan).toEqual(['packages/ui/src/components/BarraMovil.tsx']);
});

test('ningún mando de cajón solo-móvil fuera de la barra única en una app con la Shell', async () => {
  const sueltos: string[] = [];
  for await (const [rel, src] of fuentes()) {
    if (!rel.startsWith('apps/') && !rel.startsWith('packages/office/')) continue;
    if (!src.includes('SytueShell') && !rel.startsWith('packages/office/')) continue;
    const lineas = src.split('\n');
    lineas.forEach((l, i) => {
      if (!l.includes('aria-controls')) return;
      // La etiqueta del mando: unas líneas alrededor de su `aria-controls`.
      const etiqueta = lineas.slice(Math.max(0, i - 10), i + 10).join('\n');
      if (!/(^|[\s'"`])md:hidden/.test(etiqueta)) return;
      if (/SinBarraUnica|useBarraUnica|enLaTopbar/.test(src)) return;
      sueltos.push(`${rel}:${i + 1}`);
    });
  }
  expect(sueltos).toEqual([]);
});
