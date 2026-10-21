/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PDF vectorial de la escena: estructura (xref y longitudes), contenido y texto.
 */
import { expect, test } from 'bun:test';
import { aPdf, agregarNodo, anchoTexto, caminoPdf, escenaVacia } from './index.js';
import type { Escena } from './index.js';

const texto = (b: Uint8Array): string => String.fromCharCode(...b);

function completa(): Escena {
  let e = { ...escenaVacia({ ancho: 400, alto: 300 }), titulo: 'Flujo (señal)' };
  e = agregarNodo(e, 'capa1', {
    id: 'a',
    tipo: 'forma',
    forma: 'rect',
    x: 10,
    y: 10,
    w: 120,
    h: 60,
    texto: 'Inicio ñ (1)',
    rot: 20,
  });
  e = agregarNodo(e, 'capa1', {
    id: 'b',
    tipo: 'forma',
    forma: 'cylinder',
    x: 200,
    y: 10,
    w: 80,
    h: 90,
    estilo: { relleno: '#ff0000', opacidad: 0.5 },
  });
  e = agregarNodo(e, 'capa1', {
    id: 'c',
    tipo: 'conector',
    trazado: 'curved',
    desde: { nodo: 'a' },
    hasta: { nodo: 'b' },
    flechaFin: true,
  });
  e = agregarNodo(e, 'capa1', {
    id: 't',
    tipo: 'trazo',
    puntos: [0, 200, 50, 220, 100, 190, 150, 230],
  });
  e = agregarNodo(e, 'capa1', {
    id: 'x',
    tipo: 'texto',
    x: 10,
    y: 250,
    w: 300,
    h: 20,
    texto: 'Texto € “suelto”',
    negrita: true,
  });
  return e;
}

test('estructura: cabecera, xref con desplazamientos exactos y fin de fichero', () => {
  const s = texto(aPdf(completa()));
  expect(s.startsWith('%PDF-1.4')).toBe(true);
  expect(s.trimEnd().endsWith('%%EOF')).toBe(true);
  const xref = Number(/startxref\n(\d+)/.exec(s)![1]);
  expect(s.slice(xref, xref + 4)).toBe('xref');
  const filas = [...s.slice(xref).matchAll(/(\d{10}) 00000 n /g)].map((m) => Number(m[1]));
  filas.forEach((off, i) =>
    expect(s.slice(off, off + `${i + 1} 0 obj`.length)).toBe(`${i + 1} 0 obj`),
  );
  const len = Number(/\/Length (\d+)/.exec(s)![1]);
  const ini = s.indexOf('stream\n') + 7;
  expect(s.slice(ini + len, ini + len + 10)).toBe('\nendstream');
});

test('MediaBox en puntos (0,75 por unidad) y el título escapado', () => {
  const s = texto(aPdf(completa()));
  expect(s).toContain('/MediaBox [0 0 300 225]');
  expect(s).toContain('/Title (Flujo \\(se\\361al\\))');
});

test('el texto sale en WinAnsi, con paréntesis escapados y lo no representable como «?»', () => {
  const s = texto(aPdf(completa()));
  expect(s).toContain('(Inicio \\361 \\(1\\)) Tj');
  expect(s).toContain('(Texto \\200 \\223suelto\\224) Tj');
  expect(texto(aPdf({ ...completa(), titulo: '日本' }))).toContain('/Title (??)');
});

test('formas, curvas, flecha y trazo producen operadores de camino', () => {
  const s = texto(aPdf(completa()));
  expect((s.match(/ c\n/g) ?? []).length).toBeGreaterThan(10);
  expect(s).toContain('/F2 14 Tf'); // negrita
  expect(s).toMatch(/\/G\d gs/);
  expect(s).toContain('/ca 0.5 /CA 0.5');
});

test('capas ocultas no se pintan', () => {
  const e = completa();
  const oculta = { ...e, capas: e.capas.map((c) => ({ ...c, visible: false })) };
  expect(texto(aPdf(oculta))).not.toContain('Inicio');
});

test('caminoPdf: arcos de cilindro, cuadráticas y cierre', () => {
  expect(caminoPdf('M 0 0 H 10 V 5 Z')).toEqual(['0 0 m', '10 0 l', '10 5 l', 'h']);
  const q = caminoPdf('M 0 0 Q 5 10 10 0');
  expect(q[1]).toMatch(/c$/);
  const a = caminoPdf('M 0 0 A 5 5 0 0 0 10 0');
  expect(a.length).toBe(3); // m + 2 cúbicas de 90°
  expect(caminoPdf('M 1 1 Z Z')).toHaveLength(3);
});

test('anchoTexto usa la tabla de Helvetica', () => {
  expect(anchoTexto('i', 1000)).toBe(222);
  expect(anchoTexto('W', 1000)).toBe(944);
  expect(anchoTexto('W', 1000, true)).toBeCloseTo(944 * 1.06);
});
