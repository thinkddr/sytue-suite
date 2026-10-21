/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La página del documento (capa data-sytue-page de pagedecor.ts): lo que escribe el
 * importador de .docx se lee como la página del folio, y lo que pone la cinta se guarda con
 * el mismo HTML que escriben rico y el importador. Antes la página era de la vista: todo
 * documento se abría en A4 con 96 px.
 */
import { expect, test } from 'bun:test';
import {
  leerPagina,
  margenesPagina,
  medidasPagina,
  paginaHtml,
  PAGINA_POR_DEFECTO,
} from '../src/editor/pagedecor';

/** La capa con esos atributos. Sin DOM a propósito: registrar happy-dom en un fichero de
 *  test.ts cambia `fetch` y `Response` para los que vienen detrás (las pruebas del BFF). */
const capa = (html: string): Element => {
  const attrs = Object.fromEntries(
    Array.from(html.matchAll(/([\w-]+)="([^"]*)"/g), (m) => [m[1] ?? '', m[2] ?? '']),
  );
  return { getAttribute: (k: string) => attrs[k] ?? null } as unknown as Element;
};

test('la capa de un .docx importado manda en el folio', () => {
  const html =
    '<div data-sytue-page="" data-width="1056" data-height="816" data-margins="48 72 96 120" data-color="#FFF2CC" contenteditable="false" aria-hidden="true"></div>';
  const p = leerPagina(capa(html));
  expect(medidasPagina(p)).toEqual({ w: 1056, h: 816 });
  expect(p.size).toBe('carta');
  expect(p.landscape).toBe(true);
  expect(margenesPagina(p)).toEqual([48, 72, 96, 120]);
  expect(p.color).toBe('#FFF2CC');
  // Y se vuelve a escribir igual: el mismo HTML que el importador y rico.
  expect(paginaHtml(p)).toBe(html);
});

test('sin capa, la de siempre; y la de siempre no necesita capa', () => {
  expect(leerPagina(null)).toEqual(PAGINA_POR_DEFECTO);
  expect(paginaHtml(PAGINA_POR_DEFECTO)).toBe('');
  expect(paginaHtml({ ...PAGINA_POR_DEFECTO, margin: 48 })).toBe(
    '<div data-sytue-page="" data-width="794" data-height="1123" data-margins="48 48 48 48" contenteditable="false" aria-hidden="true"></div>',
  );
});

test('un tamaño propio y lo que no se entiende', () => {
  const p = leerPagina(
    capa('<div data-sytue-page="" data-width="793.73" data-height="x" data-margins="10 -5"></div>'),
  );
  expect(medidasPagina(p)).toEqual({ w: 794, h: 1123 }); // A4 a 2 px: vuelve como size
  expect(margenesPagina(p)).toEqual([10, 96, 96, 96]);
  const raro = leerPagina(
    capa('<div data-sytue-page="" data-width="600" data-height="900" data-color="red"></div>'),
  );
  expect(medidasPagina(raro)).toEqual({ w: 600, h: 900 });
  expect(raro.color).toBeUndefined();
});
