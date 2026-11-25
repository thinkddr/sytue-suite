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

test('las tareas de una nota solo pasan si quien pega las pide, y sin atributos', () => {
  const html =
    '<div class="task-row" onclick="x()"><input type="checkbox" checked onclick="y()" class="z"> ' +
    '<span style="color:red">Cajas</span></div>';
  expect(pegadoConFormato(html, { tareas: true })).toBe(
    '<div class="task-row"><input type="checkbox" checked=""> <span>Cajas</span></div>',
  );
  expect(pegadoConFormato(html)).toBe('<div> Cajas</div>');
});

test('los bloques de Notes pasan solo si quien pega los pide, y solo con sus valores', () => {
  const bloques =
    '<div data-panel="aviso" onclick="x()"><p>Ojo</p></div>' +
    '<details data-expandir open ontoggle="x()"><summary>Más</summary><p>a</p></details>' +
    '<p><span data-estado="verde" style="color:red">HECHO</span> ' +
    '<time datetime="2026-11-24">hoy</time> H<sub>2</sub>O</p>' +
    '<ul data-tipo="decisiones"><li>Sí</li></ul>' +
    '<pre data-lineas><code data-lenguaje="go">x := 1</code></pre>' +
    '<table data-numerada><thead><tr><th scope="col">A</th></tr></thead>' +
    '<tbody><tr><th scope="row">1</th><td data-fondo="azul">1</td></tr></tbody></table>' +
    '<div data-columnas="2"><div data-columna><p>c</p></div></div>';
  expect(pegadoConFormato(bloques, { bloques: true })).toBe(
    '<div data-panel="aviso"><p>Ojo</p></div>' +
      '<details data-expandir="" open=""><summary>Más</summary><p>a</p></details>' +
      '<p><span data-estado="verde">HECHO</span> <time datetime="2026-11-24">hoy</time> ' +
      'H<sub>2</sub>O</p><ul data-tipo="decisiones"><li>Sí</li></ul>' +
      '<pre data-lineas=""><code data-lenguaje="go">x := 1</code></pre>' +
      '<table data-numerada=""><thead><tr><th>A</th></tr></thead>' +
      '<tbody><tr><th scope="row">1</th><td data-fondo="azul">1</td></tr></tbody></table>' +
      '<div data-columnas="2"><div data-columna=""><p>c</p></div></div>',
  );
  // Sin pedirlos, lo de siempre: ni atributos ni etiquetas nuevas (Mail no cambia).
  expect(pegadoConFormato('<div data-panel="aviso">a</div><p>H<sub>2</sub>O</p>')).toBe(
    '<div>a</div><p>H2O</p>',
  );
});

test('un valor fuera de su lista se cae, y el atributo de un bloque no vale en otra etiqueta', () => {
  const ajeno =
    '<div data-panel="peligro">a</div><span data-estado="#f00">b</span>' +
    '<p data-estado="verde">c</p><time datetime="2026-02-30">d</time>' +
    '<pre><code data-lenguaje="cobol">e</code></pre><td data-fondo="red">f</td>' +
    '<details data-expandir="sí"><summary>g</summary></details>';
  expect(pegadoConFormato(ajeno, { bloques: true })).toBe(
    '<div>a</div>b<p>c</p><time>d</time><pre><code>e</code></pre>f' +
      '<details><summary>g</summary></details>',
  );
});

test('una fila de tarea sin `<span>` toma el resto de la fila como texto', () => {
  expect(
    pegadoConFormato('<div class="task-row"><input type="checkbox"> <b>sola</b></div>', {
      tareas: true,
    }),
  ).toBe('<div class="task-row"><input type="checkbox"> <span><b>sola</b></span></div>');
});
