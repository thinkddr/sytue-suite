/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL OSCURO TIENE DOS PUERTAS y todo lo que cambia en oscuro tiene que pasar por las dos:
 * `[data-theme='dark']` (el tema elegido a mano) y `prefers-color-scheme: dark` sobre
 * `:root:not([data-theme])` (el tema en «Sistema», que ThemeProvider aplica
 * QUITANDO `data-theme`). Es lo que hace colors.css con cada token.
 *
 * Medido el 2026-09-27 al comparar la web en oscuro con las apps nativas: tres sitios
 * entraban por una sola puerta. Las sombras de theme.css y el borde del correo maquetado
 * de Mail, solo por la elegida —en «Sistema» salían las del claro—; y el resaltado de
 * Notepads, con el `dark:` de Tailwind, solo por la del sistema —con el oscuro elegido
 * a mano salía amarillo claro sobre la página oscura—.
 *
 * Visto en ROJO con cada uno de los tres antes de arreglarlo.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const RAIZ = fileURLToPath(new URL('../../../', import.meta.url));

function fuentes(dir: string, out: string[] = []): string[] {
  for (const e of readdirSync(dir)) {
    if (e === 'node_modules' || e === 'dist' || e === '.astro' || e.endsWith('.md')) continue;
    const p = join(dir, e);
    if (statSync(p).isDirectory()) fuentes(p, out);
    else if (/\.(css|tsx|astro)$/.test(e) && !e.includes('.test.')) out.push(p);
  }
  return out;
}

const ficheros = [...fuentes(join(RAIZ, 'apps')), ...fuentes(join(RAIZ, 'packages'))];

test('ninguna clase usa el `dark:` de Tailwind, que solo mira el sistema', () => {
  const malos = ficheros.flatMap((f) =>
    [...readFileSync(f, 'utf8').matchAll(/(?<![\w-])dark:[\w[\]#/.-]+/g)].map(
      (m) => `${f}: ${m[0]}`,
    ),
  );
  expect(malos).toEqual([]);
});

test('cada regla del oscuro elegido tiene su gemela en el oscuro del sistema', () => {
  const malos: string[] = [];
  for (const f of ficheros.filter((f) => f.endsWith('.css'))) {
    const css = readFileSync(f, 'utf8')
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .replace(/\[data-theme="(dark|light|translucent)"\]/g, "[data-theme='$1']");
    const elegido = [...css.matchAll(/\[data-theme='dark'\]([^{,]*)\{/g)].map((m) => m[1]!.trim());
    const sistema = new Set(
      [...css.matchAll(/:root:not\(\[data-theme\]\)([^{,]*)\{/g)].map((m) => m[1]!.trim()),
    );
    for (const s of elegido) if (!sistema.has(s)) malos.push(`${f}: [data-theme='dark'] ${s}`);
  }
  expect(malos).toEqual([]);
});
