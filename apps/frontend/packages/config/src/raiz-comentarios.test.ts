/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS COMENTARIOS HACEN LAS CUENTAS CON LA RAÍZ DE VERDAD.
 *
 * La raíz de la suite es de 16 px (`theme.css`, y `tema.Rem` en la nativa), pero
 * muchos comentarios seguían diciendo «`h-6`… 21 px» o «`w-80` con el rem de
 * catorce: 280» como si fuera la de hoy. Quien comparaba la web con Fyne echaba
 * las cuentas con el número viejo y encontraba diferencias que no existían. Una
 * medida se escribe con la raíz de 16; una medida histórica se cuenta como pasada
 * («cuando 1 rem eran 14 px»). Es el mismo patrón que el guardián de Fyne
 * (`apps/fyne/tema/raiz_comentarios_test.go`), escrito con una clase de
 * caracteres para que este fichero no se encuentre a sí mismo.
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../', import.meta.url);
// Ojo con las llaves: Bun no expande una ruta con `/` dentro de `{…}`, y el patrón se
// quedaría sin ficheros sin decir nada. Por eso, además, se cuenta lo leído.
const FUENTES = '{apps,packages}/*/src/**/*.{ts,tsx,astro,css}';
const RAIZ_VIEJA = /rem de 1[4]|raíz.*1[4]/;

test('ningún comentario calcula con la raíz vieja', async () => {
  const malos: string[] = [];
  let leidos = 0;
  for await (const rel of new Bun.Glob(FUENTES).scan({ cwd: RAIZ.pathname })) {
    leidos++;
    const lineas = (await Bun.file(new URL(rel, RAIZ)).text()).split('\n');
    lineas.forEach((linea, i) => {
      if (RAIZ_VIEJA.test(linea)) malos.push(`${rel}:${i + 1}: ${linea.trim()}`);
    });
  }
  expect(leidos).toBeGreaterThan(1000);
  expect(malos).toEqual([]);
});
