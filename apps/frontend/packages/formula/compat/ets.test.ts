/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El suavizado exponencial de Calcus contra LibreOffice: cada fórmula del corpus
 * (ets-corpus.ts) queda dentro de la tolerancia relativa de su familia
 * (ets-divergencias.json, con su motivo), salvo las divergencias declaradas una a una. Como en
 * compat.test.ts, las dos cosas se vigilan por IGUALDAD: una fórmula que se sale sin estar
 * declarada falla, y una declarada que ya no se sale, también.
 */
import { expect, test } from 'bun:test';
import {
  compararEts,
  FAMILIAS,
  porDefecto,
  resumen,
  type EsperadoEts,
  type Tolerancias,
} from './ets-comparar';
import { DATOS_ETS, FORMULAS_ETS, SERIES } from './ets-corpus';
import divergencias from './ets-divergencias.json';
import esperado from './ets-esperado-libreoffice.json';

const TOL = divergencias as Tolerancias;
const casos = compararEts(esperado as unknown as EsperadoEts, TOL);

test('el JSON de LibreOffice es el del corpus, fórmula a fórmula y dato a dato', () => {
  expect(esperado.casos.map((c) => c.f)).toEqual(FORMULAS_ETS);
  expect<unknown>(esperado.datos).toEqual(DATOS_ETS);
  // Cada intervalo se calculó varias veces: LibreOffice no da dos veces el mismo.
  const medidos = Object.values(esperado.intervalos);
  expect(medidos.length).toBeGreaterThan(20);
  expect(medidos.every((i) => i.n >= 10)).toBe(true);
  expect(medidos.some((i) => i.max > i.min * 1.05)).toBe(true);
});

test('lo que se sale de la tolerancia de su familia está declarado, y solo eso', () => {
  const difieren = casos.filter((c) => c.veredicto === 'difiere').map((c) => c.f);
  expect(difieren.sort()).toEqual(Object.keys(TOL.divergencias).sort());
});

test('toda tolerancia y toda divergencia trae su motivo', () => {
  for (const f of FAMILIAS) expect(TOL.tolerancias[f].motivo.length, f).toBeGreaterThan(40);
  for (const [f, d] of Object.entries(TOL.divergencias)) {
    expect(d.motivo.length, f).toBeGreaterThan(40);
    expect(['libreoffice', 'modelo'], f).toContain(d.quien);
  }
});

test('con todo por defecto, el pronóstico de cada serie queda a menos del 2 % del de LibreOffice', () => {
  const todas: number[] = [];
  for (const s of SERIES) {
    const r = resumen(
      casos
        .filter((c) => c.familia === 'pronostico' && c.serie === s.nombre && porDefecto(c.f))
        .map((c) => c.rel!),
    );
    expect(r.n, s.nombre).toBeGreaterThanOrEqual(3);
    expect(r.maximo, s.nombre).toBeLessThan(0.02);
    todas.push(r.mediana);
  }
  // La mediana de las medianas por serie: lo que se publica en docs/architecture.md.
  expect(resumen(todas).mediana).toBeLessThan(0.005);
});

test('los parámetros no se comparan, pero están en [0, 1]', () => {
  const p = casos.filter((c) => c.familia === 'parametro');
  expect(p.length).toBeGreaterThan(50);
  for (const c of p) {
    expect(c.veredicto, c.f).toBe('sin-contraste');
    expect(c.nuestro as number, c.f).toBeGreaterThanOrEqual(0);
    expect(c.nuestro as number, c.f).toBeLessThanOrEqual(1);
  }
});
