/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El motor de Calcus contra LibreOffice: cada fórmula del corpus da lo que LibreOffice
 * contestó (esperado-libreoffice.json, que se regenera con libreoffice.ts), salvo las
 * divergencias declaradas en divergencias.json con su motivo. Las dos cosas se vigilan por
 * IGUALDAD y no por pertenencia: una fórmula que difiere sin estar declarada falla, y una
 * divergencia declarada que ya no existe también (hay que borrar su entrada).
 */
import { expect, test } from 'bun:test';
import { catalogo } from '../src/catalogo';
import { CASOS, DATOS, FORMULAS } from './corpus';
import { comparar, iguales, type Caso } from './comparar';
import divergencias from './divergencias.json';
import esperado from './esperado-libreoffice.json';
import type { Esperado } from './libreoffice';

const DIV = divergencias as Record<string, { quien: string; motivo: string }>;
const casos: Caso[] = comparar(esperado as Esperado);

test('el JSON de LibreOffice es el del corpus, fórmula a fórmula y dato a dato', () => {
  expect(esperado.casos.map((c) => c.f)).toEqual(FORMULAS);
  expect<unknown>(esperado.datos).toEqual(DATOS);
  expect(esperado.solo365).toEqual(
    catalogo()
      .filter((f) => f.compat === 'excel365')
      .map((f) => f.nombre),
  );
});

test('el corpus toca todas las familias y casi todo el catálogo', () => {
  expect(Object.keys(CASOS).length).toBeGreaterThanOrEqual(10);
  const usadas = new Set(
    FORMULAS.flatMap((f) => [...f.matchAll(/([A-Z][A-Z0-9.]*)\(/g)].map((m) => m[1]!)),
  );
  const sin = catalogo()
    .map((f) => f.nombre)
    .filter((n) => !usadas.has(n));
  // Las que no tienen caso se anotan aquí; no puede crecer sin que se vea.
  expect(sin.length).toBeLessThanOrEqual(110);
});

test('lo que difiere de LibreOffice está declarado, y solo eso', () => {
  const difieren = casos.filter((c) => c.veredicto === 'difiere').map((c) => c.f);
  expect(difieren.sort()).toEqual(Object.keys(DIV).sort());
});

test('toda divergencia declarada trae motivo y de quién es', () => {
  for (const [f, d] of Object.entries(DIV)) {
    expect(d.motivo.length, f).toBeGreaterThan(20);
    expect(['libreoffice', 'decidida', 'pendiente', 'falta', 'sin-referencia'], f).toContain(
      d.quien,
    );
  }
});

test('el resto de las fórmulas coincide con LibreOffice', () => {
  const malas = casos.filter((c) => c.veredicto === 'igual' && !iguales(c.nuestro, c.lo));
  expect(malas).toEqual([]);
  expect(casos.filter((c) => c.veredicto === 'igual').length).toBeGreaterThan(400);
});
