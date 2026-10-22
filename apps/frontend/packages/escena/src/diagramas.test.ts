/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Diagramas SmartArt sobre la escena: matriz, pirámide, radial y ciclo.
 */
import { expect, test } from 'bun:test';
import {
  TIPOS_DIAGRAMA,
  anadirDiagrama,
  aSvg,
  cajaMundo,
  diagrama,
  escenaVacia,
  leer,
  recorrer,
  serializar,
} from './index.js';
import type { Escena, Nodo } from './index.js';

const textos = (n: number): { text: string }[] =>
  Array.from({ length: n }, (_, i) => ({ text: `Elemento ${i + 1}` }));
const nodos = (e: Escena): Nodo[] => [...recorrer(e)].map((v) => v.nodo);
const formas = (e: Escena) => nodos(e).filter((n) => n.tipo === 'forma');
const conectores = (e: Escena) => nodos(e).filter((n) => n.tipo === 'conector');

test('todos los diseños dan una escena válida estable al serializar y leer', () => {
  for (const tipo of TIPOS_DIAGRAMA)
    for (const n of [1, 2, 3, 4, 5, 7, 12, 24]) {
      const e = diagrama(tipo, textos(n));
      const l = leer(serializar(e));
      expect(l.ok && serializar(l.escena)).toBe(serializar(e));
      expect(formas(e)).toHaveLength(n);
    }
});

test('cada elemento es una forma con su texto y nombre accesible', () => {
  const e = diagrama('matriz', [{ text: 'Fortalezas' }, { text: 'Debilidades' }]);
  expect(formas(e).map((f) => (f.tipo === 'forma' ? [f.nombre, f.texto] : []))).toEqual([
    ['Fortalezas', 'Fortalezas'],
    ['Debilidades', 'Debilidades'],
  ]);
});

test('ninguna forma se sale del lienzo y la matriz no solapa', () => {
  for (const tipo of TIPOS_DIAGRAMA)
    for (const n of [2, 3, 4, 5, 6, 9, 10]) {
      const e = diagrama(tipo, textos(n));
      for (const f of formas(e)) {
        const c = cajaMundo(e, f.id)!;
        expect(c.x).toBeGreaterThanOrEqual(-0.5);
        expect(c.y).toBeGreaterThanOrEqual(-0.5);
        expect(c.x + c.w).toBeLessThanOrEqual(e.ancho + 0.5);
        expect(c.y + c.h).toBeLessThanOrEqual(e.alto + 0.5);
      }
    }
  const m = diagrama('matriz', textos(6));
  const cs = formas(m).map((f) => cajaMundo(m, f.id)!);
  for (let i = 0; i < cs.length; i++)
    for (let j = i + 1; j < cs.length; j++) {
      const a = cs[i]!;
      const b = cs[j]!;
      expect(a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h).toBe(false);
    }
});

test('pirámide: cima triángulo y cada nivel más ancho, centrados', () => {
  const e = diagrama('piramide', textos(5));
  const fs = formas(e).flatMap((f) => (f.tipo === 'forma' ? [f] : []));
  expect(fs[0]!.forma).toBe('triangle');
  for (let i = 1; i < fs.length; i++) expect(fs[i]!.w).toBeGreaterThan(fs[i - 1]!.w);
  for (const f of fs) expect(f.x + f.w / 2).toBeCloseTo(e.ancho / 2, 0);
});

test('radial: un conector del centro a cada satélite; ciclo: flechas curvas cerrando el anillo', () => {
  const r = conectores(diagrama('radial', textos(6)));
  expect(r).toHaveLength(5);
  expect(r.every((c) => c.tipo === 'conector' && 'nodo' in c.desde && c.desde.nodo === 'n1')).toBe(
    true,
  );
  const c = conectores(diagrama('ciclo', textos(4)));
  expect(c).toHaveLength(4);
  expect(
    c.every((x) => x.tipo === 'conector' && x.trazado === 'curved' && x.flechaFin === true),
  ).toBe(true);
  const ult = c[3]!;
  expect(ult.tipo === 'conector' && 'nodo' in ult.hasta && ult.hasta.nodo).toBe('n1');
  expect(conectores(diagrama('ciclo', textos(1)))).toHaveLength(0);
});

test('sin elementos, vacía; más de 24 se recortan', () => {
  expect(formas(diagrama('matriz', []))).toHaveLength(0);
  expect(formas(diagrama('ciclo', textos(60)))).toHaveLength(24);
});

test('anadirDiagrama: capa nueva con ids únicos, conectores bien anclados y lienzo que crece', () => {
  const base = escenaVacia({ ancho: 800, alto: 300 });
  const a = anadirDiagrama(base, 'ciclo', textos(4), 'Ciclo');
  const b = anadirDiagrama(a.escena, 'ciclo', textos(4), 'Otro');
  expect(b.escena.capas.map((c) => c.nombre)).toEqual(['Capa 1', 'Ciclo', 'Otro']);
  const ids = nodos(b.escena).map((n) => n.id);
  expect(new Set(ids).size).toBe(ids.length);
  expect(b.escena.alto).toBe(420);
  expect(b.escena.ancho).toBe(800);
  const existentes = new Set(ids);
  for (const c of conectores(b.escena)) {
    if (c.tipo !== 'conector') continue;
    expect('nodo' in c.desde && existentes.has(c.desde.nodo)).toBe(true);
    expect('nodo' in c.hasta && existentes.has(c.hasta.nodo)).toBe(true);
  }
  expect(anadirDiagrama(base, 'matriz', [], 'x').escena).toBe(base);
});

test('el SVG es accesible y escapa el texto', () => {
  const svg = aSvg(
    diagrama('piramide', [{ text: '<script>x</script>' }, { text: 'B & C' }], {
      titulo: 'Pirámide',
    }),
    { responsivo: true },
  );
  expect(svg).toContain('role="img" aria-label="Pirámide"');
  expect(svg).not.toContain('<script>');
  expect(svg).toContain('B &amp; C');
});
