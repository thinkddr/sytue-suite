/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { conHashtags, conMenciones } from './hashtags.js';

const etiquetas = (html: string): string[] =>
  [...conHashtags(html).matchAll(/data-hashtag="([^"]+)"/g)].map((m) => m[1] ?? '');

// Los MISMOS casos que `TestHashtags` de services/notes/internal/domain: si la
// regla cambia allí y no aquí, se pintaría como etiqueta lo que la búsqueda no encuentra.
test('conHashtags reconoce lo mismo que el servicio', () => {
  expect(etiquetas('Llamar a #Clientes y cerrar #plan-2026 (#urgente)')).toEqual([
    'Clientes',
    'plan-2026',
    'urgente',
  ]);
  expect(etiquetas('#reunión\n#日本 al empezar línea')).toEqual(['reunión', '日本']);
  expect(etiquetas('ver https://sitio.eu/#ancla y texto#pegado')).toEqual([]);
  expect(etiquetas('it&#39;s el #1 con &#x27;comillas&#x27;')).toEqual([]);
  expect(etiquetas('fondo #ff0000 y ## Título')).toEqual([]);
  expect(etiquetas('la #decade del #plan- y #b2b')).toEqual(['decade', 'plan', 'b2b']);
});

test('conHashtags envuelve el texto y deja en paz enlaces, código y atributos', () => {
  expect(conHashtags('<p style="color:#112233">Ver <b>#plan</b>-</p>')).toBe(
    '<p style="color:#112233">Ver <b><a class="notes-hashtag" data-hashtag="plan" href="#plan">#plan</a></b>-</p>',
  );
  const intacto =
    '<p><a href="https://sytue.eu/#x">un #enlace</a> <code>#define</code></p><pre>#include</pre>';
  expect(conHashtags(intacto)).toBe(intacto);
  // Tras cerrar el enlace vuelve a mirar.
  expect(etiquetas('<a href="x">a</a> #despues')).toEqual(['despues']);
});

// El caso de `TestMencionados` de platform/pkg/hubnotify: se resalta a quien se avisa.
test('conMenciones resalta `@` y una dirección, no una dirección suelta', () => {
  expect(conMenciones('<p>@ana@x.es hola ana@x.es y (@Ana@X.es) y @luis@y.org.</p>')).toBe(
    '<p><span class="notes-mention">@ana@x.es</span> hola ana@x.es y (<span class="notes-mention">@Ana@X.es</span>) y <span class="notes-mention">@luis@y.org</span>.</p>',
  );
  const intacto = '<p><a href="mailto:ana@x.es">@ana@x.es</a> <code>@ana@x.es</code></p>';
  expect(conMenciones(intacto)).toBe(intacto);
});
