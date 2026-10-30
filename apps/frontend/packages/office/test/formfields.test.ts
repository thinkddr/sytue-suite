/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las ayudas PURAS de los campos de formulario rellenables:
 *  - buildFormFieldHtml: markup de cada tipo (FUENTE ÚNICA) con escape anti-XSS.
 *  - parseOptions: «a | b | c» → lista (recorta, sin vacíos ni duplicados).
 *  - coerceFieldValue: valida/normaliza un valor por tipo (casilla, desplegable, texto).
 *  - extractFormAnswers / answersToText: {nombre→valor} de un HTML con campos.
 * Sin DOM (bun corre sin navegador). Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/formfields.test.ts
 */
import { expect, test } from 'bun:test';
import {
  answersToText,
  buildFormFieldHtml,
  coerceFieldValue,
  extractFormAnswers,
  parseOptions,
} from '../src/editor/formfields';

test('buildFormFieldHtml: markup por tipo', () => {
  expect(buildFormFieldHtml({ type: 'text', name: 'Nombre', placeholder: 'Tu nombre' })).toBe(
    '<span data-formfield="text" data-name="Nombre" data-placeholder="Tu nombre"></span>',
  );
  expect(buildFormFieldHtml({ type: 'checkbox', name: 'Acepto', checked: true })).toBe(
    '<span data-formfield="checkbox" contenteditable="false" data-name="Acepto" data-checked="1"></span>',
  );
  expect(buildFormFieldHtml({ type: 'select', name: 'Talla', options: ['S', 'M', 'L'] })).toBe(
    '<span data-formfield="select" contenteditable="false" data-name="Talla" ' +
      'data-options="S|M|L" data-value="" data-placeholder="Elegir…"></span>',
  );
});

test('buildFormFieldHtml: escapa nombre/placeholder/opciones (anti-XSS)', () => {
  const html = buildFormFieldHtml({
    type: 'select',
    name: 'R&D "x"',
    options: ['<b>a</b>', 'b" onmouseover="alert(1)'],
  });
  expect(html).toContain('data-name="R&amp;D &quot;x&quot;"');
  expect(html).toContain('data-options="&lt;b&gt;a&lt;/b&gt;|b&quot; onmouseover=&quot;alert(1)"');
  expect(html).not.toContain('<b>a</b>');
});

test('parseOptions: recorta, descarta vacíos y duplicados', () => {
  expect(parseOptions('a|b|c')).toEqual(['a', 'b', 'c']);
  expect(parseOptions(' rojo | verde |  | rojo | azul ')).toEqual(['rojo', 'verde', 'azul']);
  expect(parseOptions('')).toEqual([]);
});

test('coerceFieldValue: casilla afirmativa/negativa', () => {
  expect(coerceFieldValue('checkbox', 'sí')).toBe('1');
  expect(coerceFieldValue('checkbox', '1')).toBe('1');
  expect(coerceFieldValue('checkbox', 'X')).toBe('1');
  expect(coerceFieldValue('checkbox', 'no')).toBe('0');
  expect(coerceFieldValue('checkbox', '')).toBe('0');
});

test('coerceFieldValue: desplegable valida contra las opciones', () => {
  const opts = ['S', 'M', 'L'];
  expect(coerceFieldValue('select', 'M', opts)).toBe('M');
  expect(coerceFieldValue('select', '', opts)).toBe(''); // vaciar es válido
  expect(coerceFieldValue('select', 'XL', opts)).toBeNull(); // no está en la lista
});

test('coerceFieldValue: texto pasa tal cual', () => {
  expect(coerceFieldValue('text', 'hola mundo')).toBe('hola mundo');
});

test('extractFormAnswers: recolecta {nombre, tipo, valor} en orden', () => {
  const html =
    '<p>Nombre: <span data-formfield="text" data-name="Nombre" data-placeholder="Tu nombre">Ana</span></p>' +
    '<p>Acepto <span data-formfield="checkbox" contenteditable="false" data-name="Acepto" data-checked="1"></span></p>' +
    '<p>Sin marcar <span data-formfield="checkbox" contenteditable="false" data-name="Boletín" data-checked="0"></span></p>' +
    '<p>Talla <span data-value="M" data-formfield="select" contenteditable="false" data-name="Talla" data-options="S|M|L" data-placeholder="Elegir…"></span></p>';
  expect(extractFormAnswers(html)).toEqual([
    { name: 'Nombre', type: 'text', value: 'Ana' },
    { name: 'Acepto', type: 'checkbox', value: 'Sí' },
    { name: 'Boletín', type: 'checkbox', value: 'No' },
    { name: 'Talla', type: 'select', value: 'M' }, // atributos en otro orden: sigue funcionando
  ]);
});

test('extractFormAnswers: campo de texto vacío y desescape de nombre/valor', () => {
  const html =
    '<span data-formfield="text" data-name="R&amp;D" data-placeholder="x"><br></span>' +
    '<span data-formfield="text" data-name="Cita" data-placeholder="x">dijo &quot;hola&quot;</span>';
  expect(extractFormAnswers(html)).toEqual([
    { name: 'R&D', type: 'text', value: '' }, // <br> del contenteditable vacío no cuenta
    { name: 'Cita', type: 'text', value: 'dijo "hola"' },
  ]);
});

test('answersToText: «Nombre: valor» por línea', () => {
  const answers = extractFormAnswers(
    '<span data-formfield="text" data-name="Nombre" data-placeholder="x">Ana</span>' +
      '<span data-formfield="checkbox" data-name="Acepto" data-checked="1"></span>',
  );
  expect(answersToText(answers)).toBe('Nombre: Ana\nAcepto: Sí');
});

test('extractFormAnswers: parsea atributos con >, comillas simples y spans anidados', () => {
  expect(
    extractFormAnswers(
      "<span title='a > b' data-name='Nombre' data-formfield='text'>Ana <span><b>María</b></span> López</span>",
    ),
  ).toEqual([{ name: 'Nombre', type: 'text', value: 'Ana María López' }]);
});

test('extractFormAnswers: no reconstruye etiquetas a partir de marcado malformado', () => {
  expect(
    extractFormAnswers(
      '<span data-formfield="text" data-name="x"><<script>script>alert(1)</script>fin</span>',
    ),
  ).toEqual([{ name: 'x', type: 'text', value: '<fin' }]);
});

test('extractFormAnswers: ignora scripts, estilos, plantillas y campos en comentarios', () => {
  const fake = '<span data-formfield="text" data-name="falso">oculto</span>';
  expect(
    extractFormAnswers(
      `<!--${fake}--><script>${fake}</script><template>${fake}</template>` +
        '<span data-formfield="text" data-name="real">antes<script>alert(1)</script><style>body{}</style>después</span>',
    ),
  ).toEqual([{ name: 'real', type: 'text', value: 'antesdespués' }]);
});

test('extractFormAnswers: decodifica entidades de nombres y valores una sola vez', () => {
  expect(
    extractFormAnswers(
      '<span data-formfield="text" data-name="&#78;ombre">&amp;lt;script&amp;gt; &#x1f600; &lt;b&gt;</span>' +
        '<span data-formfield="select" data-name="Opción" data-value="&amp;lt; &quot;x&quot;"></span>',
    ),
  ).toEqual([
    { name: 'Nombre', type: 'text', value: '&lt;script&gt; 😀 <b>' },
    { name: 'Opción', type: 'select', value: '&lt; "x"' },
  ]);
});

test('extractFormAnswers: no confunde atributos prefijados con campos reales', () => {
  expect(extractFormAnswers('<span other-data-formfield="text" data-name="x">no</span>')).toEqual(
    [],
  );
});
