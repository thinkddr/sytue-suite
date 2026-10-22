/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los plurales de TODOS los catálogos (el común y el de cada app) se eligen con
 * `Intl.PluralRules`, y cada idioma tiene sus categorías: el irlandés y el maltés
 * cinco, el lituano cuatro. Dos errores que esto atrapa y que ya estuvieron ahí:
 *
 *  - una forma con un nombre que ese idioma no tiene (se ignora sin avisar);
 *  - la forma del 1 escrita en singular sin `{count}` («la única página») cuando esa
 *    categoría cubre más números: en lituano `one` es también el 21, el 31…;
 *  - un «1» escrito a mano en una categoría que cubre otros números, el 0 incluido:
 *    en francés `one` es también el 0, y salía «1 ligne» para ninguna fila.
 *
 * Las demás formas sin `{count}` («¿Borrar estos archivos?») son plurales de verdad y
 * valen para todos sus números, y una forma idéntica a `other` vale para cualquiera.
 *
 * Se descubren las apps leyendo el árbol, como en catalogo-por-app.test.ts: una app
 * nueva entra sola.
 */
import { describe, expect, test } from 'bun:test';
import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

const FRONTEND = join(import.meta.dir, '..', '..', '..');
const carpetas = [
  join(import.meta.dir, 'messages'),
  // La ofimática (Docus, Calcus, Diapos y CertiPDF) lleva su catálogo en su paquete.
  join(FRONTEND, 'packages', 'office', 'src/i18n/messages'),
  ...readdirSync(join(FRONTEND, 'apps'))
    .map((a) => join(FRONTEND, 'apps', a, 'src/i18n/messages'))
    .filter(existsSync),
];
const catalogos = carpetas.flatMap((d) =>
  readdirSync(d)
    .filter((f) => /^[a-z]{2}\.ts$/.test(f))
    .map((f) => [join(d, f), f.slice(0, 2)] as const),
);

/** Qué enteros del 0 al 1000 caen en cada categoría de ese idioma. */
const enteros = (l: string): Map<string, number[]> => {
  const reglas = new Intl.PluralRules(l);
  const m = new Map<string, number[]>();
  for (let n = 0; n <= 1000; n++) m.set(reglas.select(n), [...(m.get(reglas.select(n)) ?? []), n]);
  return m;
};

describe('plurales de todos los catálogos', () => {
  test('se encuentran los catálogos de las apps', () => {
    expect(carpetas.length).toBeGreaterThanOrEqual(14);
  });

  test.each(catalogos.map(([ruta, l]) => [ruta.slice(FRONTEND.length + 1), l, ruta]))(
    '%s: formas que existen y números que no mienten',
    async (_nombre, l, ruta) => {
      const mod = (await import(ruta)) as Record<string, Record<string, unknown>>;
      const catalogo = mod[l]!;
      const categorias = new Intl.PluralRules(l).resolvedOptions().pluralCategories as string[];
      const cubre = enteros(l);
      const mal: string[] = [];
      for (const [clave, formas] of Object.entries(catalogo)) {
        if (typeof formas !== 'object' || formas === null) continue;
        for (const [forma, texto] of Object.entries(formas as Record<string, string>)) {
          if (!categorias.includes(forma)) mal.push(`${clave}.${forma}: «${l}» no tiene esa forma`);
          // Igual que `other`: el mismo texto vale para cualquier número.
          if (texto.includes('{count}') || texto === (formas as Record<string, string>)['other'])
            continue;
          const n = cubre.get(forma) ?? [];
          const otro = n.find((x) => x >= 2);
          if (n.includes(1) && otro !== undefined)
            mal.push(
              `${clave}.${forma}: «${texto}» es la forma del 1 sin {count}, y vale también para ${otro}`,
            );
          else if (/(^|\D)1(\D|$)/.test(texto) && n.some((x) => x !== 1))
            mal.push(
              `${clave}.${forma}: «${texto}» lleva el 1 a mano, y ${forma} es también ${n.find((x) => x !== 1)}`,
            );
        }
      }
      expect(mal).toEqual([]);
    },
  );
});
