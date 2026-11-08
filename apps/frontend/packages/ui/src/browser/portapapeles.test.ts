/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import { htmlATexto, pegadoConFormato, textoPegado } from './portapapeles.js';

let registrado = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registrado = true;
  }
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const portapapeles = (tipos: Record<string, string>) => ({
  getData: (t: string) => tipos[t] ?? '',
});

test('con texto plano manda el texto plano, aunque venga HTML', () => {
  expect(textoPegado(portapapeles({ 'text/plain': 'plano', 'text/html': '<b>rico</b>' }))).toBe(
    'plano',
  );
});

test('Safari: solo `text/html` también pega, como texto', () => {
  const html =
    '<meta charset="utf-8"><style>p{color:red}</style><p>Hola <b>mundo</b></p><p>dos</p>';
  expect(textoPegado(portapapeles({ 'text/html': html }))).toBe('Hola mundo\ndos');
});

test('sin nada que pegar devuelve vacío', () => {
  expect(textoPegado(portapapeles({}))).toBe('');
  expect(textoPegado(null)).toBe('');
});

test('el HTML pasa a texto con sus saltos y sin marcado', () => {
  expect(htmlATexto('<p>uno</p><p>dos</p>')).toBe('uno\ndos');
  expect(htmlATexto('a<br>b')).toBe('a\nb');
  expect(htmlATexto('<script>alert(1)</script>x')).toBe('x');
  expect(htmlATexto('<table><tr><td>a</td></tr></table>')).toBe('a');
});

test('htmlATexto ignora marcado malformado, plantillas y comentarios repetidos en tiempo lineal', () => {
  expect(htmlATexto('<<script>script>alert(1)</script>fin')).toBe('<fin');
  expect(htmlATexto('a<!--oculto--><template>no</template><noscript>no</noscript>b')).toBe('ab');
  expect(htmlATexto('<p>uno   \t</p><p>dos</p>')).toBe('uno\ndos');
  const muchosComentarios = '<!--'.repeat(4000) + 'fin-->hola';
  expect(htmlATexto(muchosComentarios)).toBe('hola');
});

// --- pegar con formato: estructura del origen, estilo del destino -------------

test('conserva títulos, negrita, cursiva, listas y enlaces, sin el estilo del origen', () => {
  const html =
    '<h1 style="font-family:Comic Sans;color:red" class="x">Título</h1>' +
    '<p style="font-size:30px">Hola <strong>fuerte</strong> y <em>suave</em> ' +
    '<a href="https://sytue.eu" style="color:blue" target="_blank">enlace</a></p>' +
    '<ul><li>uno</li></ul><h6>bajo</h6>';
  expect(pegadoConFormato(html)).toBe(
    '<h1>Título</h1><p>Hola <b>fuerte</b> y <i>suave</i> <a href="https://sytue.eu/">enlace</a></p>' +
      '<ul><li>uno</li></ul><h5>bajo</h5>',
  );
});

test('Google Docs: el envoltorio sin negrita no pone todo en negrita y el `style` cuenta', () => {
  const html =
    '<meta charset="utf-8"><b style="font-weight:normal;" id="docs-internal-guid-1">' +
    '<p><span style="font-weight:700;font-family:Arial">negrita</span> ' +
    '<span style="font-style:italic;text-decoration:underline">cursiva</span></p></b>';
  expect(pegadoConFormato(html)).toBe('<p><b>negrita</b> <i><u>cursiva</u></i></p>');
});

test('nada que ejecute ni cargue sobrevive', () => {
  const html =
    '<p onclick="alert(1)">a<img src=x onerror="alert(2)"><script>alert(3)</script>' +
    '<a href="javascript:alert(4)">b</a><style>p{}</style><iframe src="https://x"></iframe></p>';
  const limpio = pegadoConFormato(html);
  expect(limpio).toBe('<p>ab</p>');
});

test('el salto final que añade Safari no se pega', () => {
  expect(pegadoConFormato('<p>x</p><br class="Apple-interchange-newline">')).toBe('<p>x</p>');
});

test('sin texto no hay nada que pegar', () => {
  expect(pegadoConFormato('<img src="x">')).toBe('');
  expect(pegadoConFormato('')).toBe('');
});
