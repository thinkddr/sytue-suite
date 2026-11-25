/**
 * Las imágenes y los ficheros del texto de una nota: lo que entra al ponerlos, cómo se
 * pintan al escribir y al leer, y que lo que no es un adjunto de la nota —o no es del
 * tipo que dice— no se pinta (adjuntos-texto.ts). Y la barra pegada al teclado.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import { htmlDeAdjunto, pintarAdjuntos } from './adjuntos-texto.js';
import { MINIMO_TECLADO, tapaDelTeclado } from './barra-teclado.js';

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

const PNG = {
  id: 'a1',
  name: 'plano <1>.png',
  size: 4,
  type: 'image/png',
  dataUrl: 'data:image/png;base64,iVBO',
};
const PDF = {
  id: 'a2',
  name: 'informe.pdf',
  size: 4,
  type: 'application/pdf',
  dataUrl: 'data:application/pdf;base64,JVBERg==',
};

test('una imagen entra como imagen y lo demás como enlace a su fichero, escapados', () => {
  expect(htmlDeAdjunto(PNG)).toBe('<img src="adjunto:a1" alt="plano &lt;1&gt;.png">');
  expect(htmlDeAdjunto(PDF)).toBe('<a href="adjunto:a2">informe.pdf</a>&nbsp;');
  expect(htmlDeAdjunto({ ...PNG, id: 'x" onerror="y' })).toBe('');
});

test('al leer, la imagen toma sus datos y el fichero se descarga con su nombre', () => {
  const campo = document.createElement('div');
  campo.innerHTML =
    '<p><img src="adjunto:a1"><a href="adjunto:a2" target="_blank" rel="noopener noreferrer">f</a>' +
    '<img src="adjunto:zz"><a href="adjunto:zz">g</a></p>';
  pintarAdjuntos(campo, [PNG, PDF], true);
  const [img, rota] = [...campo.querySelectorAll('img')];
  const [enlace, suelto] = [...campo.querySelectorAll('a')];
  // Sus datos, en una URL `blob:` corta (o en la de datos donde no las hay).
  expect(img!.getAttribute('src')).toMatch(/^(blob:|data:image\/png;)/);
  expect(rota!.getAttribute('src')).toBe('adjunto:zz');
  expect(enlace!.getAttribute('href')).toMatch(/^(blob:|data:application\/pdf;)/);
  expect(enlace!.getAttribute('download')).toBe('informe.pdf');
  expect(enlace!.hasAttribute('target')).toBe(false);
  expect(suelto!.getAttribute('href')).toBe('adjunto:zz');
});

test('un adjunto que no es del tipo que dice, o no es una URL de datos, no se pinta', () => {
  const campo = document.createElement('div');
  campo.innerHTML = '<img src="adjunto:a1"><img src="adjunto:a3">';
  pintarAdjuntos(
    campo,
    [
      { ...PNG, dataUrl: 'data:image/svg+xml;base64,PHN2Zz4=' },
      { ...PNG, id: 'a3', dataUrl: 'javascript:alert(1)' },
    ],
    false,
  );
  expect([...campo.querySelectorAll('img')].map((i) => i.getAttribute('src'))).toEqual([
    'adjunto:a1',
    'adjunto:a3',
  ]);
});

test('la barra va pegada al teclado solo con el teclado fuera', () => {
  expect(tapaDelTeclado(800, { height: 800, offsetTop: 0 })).toBeNull();
  expect(tapaDelTeclado(800, { height: 800 - MINIMO_TECLADO + 1, offsetTop: 0 })).toBeNull();
  expect(tapaDelTeclado(800, { height: 480, offsetTop: 0 })).toBe(320);
  // Con la página desplazada dentro del viewport visual (iOS), lo que tapa es menos.
  expect(tapaDelTeclado(800, { height: 480, offsetTop: 40 })).toBe(280);
  expect(tapaDelTeclado(800, null)).toBeNull();
});
