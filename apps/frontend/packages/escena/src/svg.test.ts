/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Exportación a SVG: estructura, accesibilidad, escapado y autonomía.
 */
import { expect, test } from 'bun:test';
import {
  agregarCapa,
  agregarNodo,
  ajustarCapa,
  ajustarTexto,
  aSvg,
  escenaVacia,
  sanear,
  trazoPath,
} from './index.js';
import type { Escena, Nodo } from './index.js';

function con(...nodos: Nodo[]): Escena {
  let e = { ...escenaVacia({ ancho: 400, alto: 300 }), titulo: 'Mi dibujo' };
  for (const n of nodos) e = agregarNodo(e, 'capa1', n);
  return e;
}

test('cabecera: viewBox, tamaño, role e aria-label', () => {
  const s = aSvg(con());
  expect(s).toStartWith('<svg xmlns="http://www.w3.org/2000/svg"');
  expect(s).toContain('viewBox="0 0 400 300" width="400" height="300"');
  expect(s).toContain('role="img" aria-label="Mi dibujo"');
  expect(s).toContain('<title>Mi dibujo</title>');
  expect(aSvg(con(), { responsivo: true })).toContain(
    'width="100%" style="display:block;height:auto"',
  );
  const sin = aSvg({ ...con(), titulo: undefined } as unknown as Escena);
  expect(sin).not.toContain('role="img"');
});

test('es autónomo: sin defs, foreignObject, script ni eventos', () => {
  const s = aSvg(
    con(
      { id: 'a', tipo: 'forma', forma: 'star', x: 0, y: 0, w: 80, h: 80, texto: 'x' },
      { id: 'k', tipo: 'linea', x1: 0, y1: 0, x2: 9, y2: 9, flechaIni: true, flechaFin: true },
    ),
  );
  for (const malo of ['<defs', 'foreignObject', '<script', ' on', 'javascript:']) {
    expect(s.includes(malo)).toBe(false);
  }
});

test('escapa texto, nombres y alternativas de los usuarios', () => {
  const s = aSvg(
    con(
      {
        id: 'a',
        tipo: 'forma',
        forma: 'rect',
        x: 0,
        y: 0,
        w: 500,
        h: 50,
        texto: '<b>"&"</b>',
        nombre: 'a"><script>',
      },
      {
        id: 'i',
        tipo: 'imagen',
        x: 0,
        y: 0,
        w: 5,
        h: 5,
        href: '/x.png?a=1&b="2"',
        alt: '"><svg onload=1>',
      },
    ),
  );
  expect(s).not.toContain('<script>');
  expect(s).not.toContain('<b>');
  expect(s).toContain('&lt;b&gt;&quot;&amp;&quot;&lt;/b&gt;');
  expect(s).toContain('aria-label="&quot;&gt;&lt;svg onload=1&gt;"');
  expect(s).toContain('href="/x.png?a=1&amp;b=&quot;2&quot;"');
});

test('quita los caracteres que XML no admite', () => {
  expect(
    aSvg(con({ id: 't', tipo: 'texto', x: 0, y: 0, w: 100, h: 20, texto: 'a\u0000b\u0008c' })),
  ).toContain('>abc<');
});

test('orden de capas, capas ocultas y marcas data-id', () => {
  let e = con({ id: 'a', tipo: 'forma', forma: 'rect', x: 0, y: 0, w: 5, h: 5 });
  e = agregarCapa(e, { id: 'c2', nombre: 'Notas' });
  e = agregarNodo(e, 'c2', { id: 'b', tipo: 'forma', forma: 'rect', x: 0, y: 0, w: 5, h: 5 });
  const s = aSvg(e);
  expect(s.indexOf('data-id="a"')).toBeLessThan(s.indexOf('data-id="b"'));
  expect(s).toContain('aria-label="Notas"');
  expect(aSvg(ajustarCapa(e, 'c2', { visible: false }))).not.toContain('data-id="b"');
  expect(aSvg(e, { marcas: false })).not.toContain('data-id');
});

test('los nodos con nombre llevan <title>; los giros salen en la matriz', () => {
  const s = aSvg(
    con({
      id: 'a',
      tipo: 'forma',
      forma: 'rect',
      x: 10,
      y: 20,
      w: 100,
      h: 50,
      rot: 90,
      nombre: 'Caja de entrada',
    }),
  );
  expect(s).toContain('<title>Caja de entrada</title>');
  expect(s).toMatch(/transform="matrix\(0 1 -1 0 85 -?\d+(\.\d+)?\)"/);
});

test('sin color fijado toma la tinta del tema; con color, el suyo', () => {
  const s = aSvg(
    con(
      { id: 'a', tipo: 'forma', forma: 'rect', x: 0, y: 0, w: 5, h: 5 },
      {
        id: 'b',
        tipo: 'forma',
        forma: 'rect',
        x: 0,
        y: 0,
        w: 5,
        h: 5,
        estilo: { relleno: '#aa0000', trazo: '#00aa00', grosor: 4 },
      },
    ),
  );
  expect(s).toContain('fill="currentColor" fill-opacity="0.08"');
  expect(s).toContain('fill="#aa0000" stroke="#00aa00" stroke-width="4"');
});

test('el conector dibuja su ruta y las puntas', () => {
  const s = aSvg(
    con(
      { id: 'a', tipo: 'forma', forma: 'rect', x: 0, y: 0, w: 100, h: 50 },
      { id: 'b', tipo: 'forma', forma: 'rect', x: 300, y: 0, w: 100, h: 50 },
      {
        id: 'k',
        tipo: 'conector',
        trazado: 'straight',
        desde: { nodo: 'a' },
        hasta: { nodo: 'b' },
        flechaFin: true,
      },
    ),
  );
  expect(s).toContain('d="M 100 25 L 300 25"');
  expect(s.match(/<path /g)!.length).toBeGreaterThanOrEqual(2); // ruta + punta
});

test('aSvg es determinista y no cambia con la lectura/escritura', () => {
  const e = con({ id: 'p', tipo: 'trazo', puntos: [0, 0, 5, 5, 10, 0] });
  const r = sanear(JSON.parse(JSON.stringify(e)));
  expect(r.ok && aSvg(r.escena)).toBe(aSvg(e));
});

test('ajustarTexto respeta saltos y parte lo que no cabe', () => {
  expect(ajustarTexto('uno dos tres', 1000, 14)).toEqual(['uno dos tres']);
  expect(ajustarTexto('a\n\nb', 1000, 14)).toEqual(['a', '', 'b']);
  const l = ajustarTexto('palabra larguísima de verdad', 56, 14); // ~7 caracteres por línea
  expect(l.every((x) => x.length <= 7)).toBe(true);
  expect(l.join('').replace(/\s/g, '')).toBe('palabralarguísimadeverdad');
});

test('trazoPath: punto, segmento y curva suave', () => {
  expect(trazoPath([5, 5])).toBe('M 5 5 L 5 5');
  expect(trazoPath([0, 0, 10, 10])).toBe('M 0 0 L 10 10');
  expect(trazoPath([0, 0, 10, 10, 20, 0])).toBe('M 0 0 Q 10 10 15 5 L 20 0');
  expect(trazoPath([])).toBe('');
});
