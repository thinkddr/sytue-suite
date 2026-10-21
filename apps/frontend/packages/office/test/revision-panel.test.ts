/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del «cuándo» del panel de revisión (src/editor/panels/RevisionPanel.tsx):
 * el sello de tiempo de cada marca sale de su `data-ts`, y ese atributo llega del HTML
 * del folio —FRONTERA DE CONFIANZA, viaja en un documento compartido—. trackTime es el
 * guardián: acepta sellos reales y devuelve null ante cualquier basura, para que un
 * `data-ts` inventado no reviente el render con un `new Date(...).toISOString()`.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/revision-panel.test.ts
 */
import { expect, test } from 'bun:test';
import { trackTime } from '../src/editor/panels/RevisionPanel';
import { insertionHtml } from '../src/editor/trackchanges';

test('trackTime acepta el sello que hornea la propia marca', () => {
  const now = Date.now();
  // El mismo valor que insertionHtml escribe en data-ts: round-trip del «cuándo».
  const raw = /data-ts="([^"]*)"/.exec(insertionHtml('x', { name: 'Ana', id: 'u1' }, now))?.[1];
  expect(trackTime(raw)).toBe(now);
  expect(new Date(trackTime(raw) ?? 0).toISOString()).toContain('T'); // el render no estalla
});

test('trackTime devuelve null si falta el sello (marcas anteriores al «cuándo»)', () => {
  expect(trackTime(undefined)).toBeNull();
  expect(trackTime('')).toBeNull(); // Number('') === 0: 1970 sería una fecha FALSA
  expect(trackTime('0')).toBeNull();
});

test('trackTime rechaza sellos que romperían el render (data-ts hostil)', () => {
  for (const hostil of ['ayer', 'NaN', '-1', '1e400', String(1e20), '8.64e15']) {
    const ts = trackTime(hostil);
    // O null, o una fecha que toISOString() sabe serializar: nunca un RangeError.
    if (ts !== null) expect(() => new Date(ts).toISOString()).not.toThrow();
    else expect(ts).toBeNull();
  }
  // Fuera del rango representable de Date (±8.64e15 ms): sin fecha, no excepción.
  expect(trackTime(String(1e20))).toBeNull();
  expect(trackTime('1e400')).toBeNull(); // Infinity
});
