import { expect, test } from 'bun:test';
import { indiceHtmlDeTexto, longitudVisible, offsetDeIndiceHtml } from './textohtml.js';

const HTML = '<p class="a>b">Hola <b>mun</b>do &amp; más</p><!-- x > y --><p>é😀</p>';
// texto visible: «Hola mundo & más» + «é😀»  (= 5+3+2+... ver abajo)

test('longitudVisible: cuenta solo el texto, con referencias y pares suplentes', () => {
  expect(longitudVisible('<p>ab</p>')).toBe(2);
  expect(longitudVisible('a&amp;b&#128512;')).toBe(1 + 1 + 1 + 2);
  expect(longitudVisible(HTML)).toBe('Hola mundo & más'.length + 'é😀'.length);
  expect(longitudVisible('<img alt="a>b"><br>')).toBe(0);
});

test('texto ↔ índice del HTML: ida y vuelta, saltando etiquetas', () => {
  const html = '<p>ab<b>cd</b>ef</p>';
  // «abcdef»: tras 2 caracteres visibles, justo después de la b (antes de <b>)
  expect(indiceHtmlDeTexto(html, 0)).toBe(0);
  expect(indiceHtmlDeTexto(html, 2)).toBe('<p>ab'.length);
  expect(indiceHtmlDeTexto(html, 4)).toBe('<p>ab<b>cd'.length);
  expect(indiceHtmlDeTexto(html, 6)).toBe('<p>ab<b>cd</b>ef'.length);
  expect(indiceHtmlDeTexto(html, 7)).toBeNull(); // más allá del final
  for (let off = 0; off <= 6; off++) {
    expect(offsetDeIndiceHtml(html, indiceHtmlDeTexto(html, off)!)).toBe(off);
  }
});

test('texto ↔ índice: el par suplente no se parte', () => {
  const html = '<p>a😀b</p>';
  expect(indiceHtmlDeTexto(html, 2)).toBeNull(); // a mitad del emoji
  expect(indiceHtmlDeTexto(html, 3)).toBe(Array.from('<p>a😀').length);
});
