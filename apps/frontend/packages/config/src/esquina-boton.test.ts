/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA ESQUINA DE UN BOTÓN LA PONE `Button`: `rounded-control`, 4 px, compartido con
 * Fyne (`tema.Forma("radius-control")`). Las clases de una app no deben fijar otra
 * esquina: los `ACTION_BTN` y `TEXT_BTN` de Docs ya la habían divergido al añadir
 * `rounded-sm` a una clase que solo existía para atenuar el color.
 *
 * Mira las constantes de clase con `rounded-sm` que acaban en el `class` de un
 * `<Button>` o `<IconButton>`, y las `class="…rounded-sm…"` escritas en uno.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const APPS = fileURLToPath(new URL('../../../apps/', import.meta.url));
/** La ofimática común es código de app aunque viva en un paquete (`packages/office`). */
const OFICINA = fileURLToPath(new URL('../../office/src/', import.meta.url));
const FRONTEND = fileURLToPath(new URL('../../../', import.meta.url));

function tsx(dir: string, out: string[] = []): string[] {
  for (const e of readdirSync(dir)) {
    if (e === 'node_modules' || e === 'dist' || e === '.astro') continue;
    const p = join(dir, e);
    if (statSync(p).isDirectory()) tsx(p, out);
    else if (e.endsWith('.tsx') && !e.includes('.test.')) out.push(p);
  }
  return out;
}

/** El nombre de la etiqueta JSX que contiene la posición `i`. */
function etiquetaEn(src: string, i: number): string {
  const abre = src.lastIndexOf('<', i);
  return /^<([A-Za-z.]+)/.exec(src.slice(abre))?.[1] ?? '';
}

test('ningún Button ni IconButton pisa su esquina con rounded-sm', () => {
  const malos: string[] = [];
  for (const f of [...tsx(APPS), ...tsx(OFICINA)]) {
    const src = readFileSync(f, 'utf8');
    const constantes = [...src.matchAll(/const (\w+) = ['"`]([^'"`]*)['"`]/g)]
      .filter((m) => /\brounded-sm\b/.test(m[2] ?? ''))
      .map((m) => m[1]);
    for (const c of constantes) {
      for (const m of src.matchAll(new RegExp(`class=\\{${c}\\}`, 'g'))) {
        if (/^(Button|IconButton)$/.test(etiquetaEn(src, m.index))) malos.push(`${f}: ${c}`);
      }
    }
    for (const m of src.matchAll(/class="[^"]*\brounded-sm\b[^"]*"/g)) {
      if (/^(Button|IconButton)$/.test(etiquetaEn(src, m.index))) malos.push(`${f}: ${m[0]}`);
    }
  }
  expect(malos.map((m) => m.slice(FRONTEND.length))).toEqual([]);
});
