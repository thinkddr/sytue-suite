/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA ETIQUETA DE UN RECURSO ES `Tag`, NO UNA PÍLDORA COPIADA.
 *
 * Calendar (invitados), Contacts (grupos) y Mail (etiquetas de un hilo) se escribían cada
 * una su chip con la misma receta —`rounded-pill`/`rounded-full`, `border`, `pr-1 pl-2.5`
 * y un `<button>` con el aspa— y dos de las tres se quedaron con un aspa de 20 px, por
 * debajo de los 24 de WCAG 2.5.8 y sin suelo táctil: copiar las clases del botón no copia
 * su diana. `Tag` trae el aspa de `IconButton sm` (24 px, 44 con el dedo).
 *
 * SI ESTE TEST TE PARA: usa `<Tag>` de `@sytue/ui` (con `onRemove` + `removeLabel` si se
 * puede quitar, `dot` para el punto de color y `trailing` para lo que acompaña al texto).
 * Las excepciones son chips que NO son la etiqueta de un recurso, y se comparan POR
 * IGUALDAD: si una deja de serlo, bórrala de aquí.
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../../', import.meta.url).pathname; // apps/frontend/

/** Fichero → por qué su píldora no es un `Tag`. Ruta relativa a `apps/frontend/`. */
const EXCEPCIONES: Record<string, string> = {
  'apps/mail/src/components/AddressField.tsx':
    'la ficha de un destinatario es parte de un campo de texto: alto fijo de 7 para alinearse con el cursor, estado de dirección no válida en rojo y un aspa que no roba el foco al campo (mousedown sin blur)',
};

/**
 * La receta de las copias en una misma cadena de clases: la píldora redonda con el lado
 * derecho estrecho para el aspa (`pr-1`/`pe-1`) y el izquierdo de chip (`pl-2.5`,
 * `ps-2.5`, `ps-2`…).
 */
function esReceta(clases: string): boolean {
  const c = clases.split(/\s+/);
  const redonda = c.some((x) => /^rounded-(pill|full)$/.test(x));
  const derecha = c.some((x) => x === 'pr-1' || x === 'pe-1');
  const izquierda = c.some((x) => /^(pl|ps)-2(\.5)?$/.test(x));
  return redonda && derecha && izquierda;
}

/** Las cadenas de clases del código: los `class="…"` y los literales entre comillas. */
function cadenas(codigo: string): string[] {
  return [...codigo.matchAll(/(["'`])((?:(?!\1)[^\\\n]|\\.)*)\1/g)].map((m) => m[2]!);
}

test('las etiquetas de un recurso salen de Tag, sin recetas locales', async () => {
  const copias: Record<string, string> = {};
  const glob = new Bun.Glob('{apps,packages}/*/src/**/*.tsx');
  for await (const rel of glob.scan({ cwd: RAIZ })) {
    if (rel.includes('.test.') || rel === 'packages/ui/src/components/Tag.tsx') continue;
    const codigo = await Bun.file(RAIZ + rel).text();
    if (cadenas(codigo).some(esReceta))
      copias[rel] = EXCEPCIONES[rel] ?? 'SIN MOTIVO: usa <Tag> de @sytue/ui';
  }
  expect(copias).toEqual(EXCEPCIONES);
});

test('el detector reconoce las tres recetas que se retiraron', () => {
  // Las de EventModal, ContactDetail y MessageReader tal como estaban.
  expect(
    esReceta('rounded-pill border-border bg-bg-subtle inline-flex border py-0.5 pr-1 pl-2.5'),
  ).toBe(true);
  expect(esReceta('bg-bg-subtle inline-flex rounded-full border py-0.5 pr-1 pl-2.5')).toBe(true);
  expect(esReceta('rounded-pill inline-flex gap-1.5 border py-0.5 ps-2 pe-1 text-xs')).toBe(true);
  // Un punto redondo o un botón píldora no son la receta.
  expect(esReceta('h-2 w-2 shrink-0 rounded-full')).toBe(false);
  expect(esReceta('rounded-pill px-3 py-1')).toBe(false);
});
