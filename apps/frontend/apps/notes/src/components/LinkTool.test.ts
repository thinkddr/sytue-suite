/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La parte de Drive de la herramienta de enlaces. Enseñaba siempre lo primero de
 * «Mi unidad» —carpetas incluidas— y a lo que no traía enlace le inventaba uno,
 * `/drive?file=…`, relativo al host de Notepads: un 404 guardado en la nota.
 */
import { expect, test } from 'bun:test';
import { opcionesDrive } from './LinkTool.js';

test('solo se ofrecen ficheros con su enlace: ni carpetas ni enlaces inventados', () => {
  const op = opcionesDrive([
    { id: 'k', name: 'Actas', isDirectory: true, path: '/Actas' },
    { id: 'f1', name: 'acta.pdf', url: 'https://drive.sytue.eu/drive/files/f1' },
    { id: 'f2', name: 'sin-enlace.pdf' },
  ]);
  expect(op.map((o) => [o.label, o.url])).toEqual([
    ['acta.pdf', 'https://drive.sytue.eu/drive/files/f1'],
  ]);
});

test('como mucho ocho, después de quitar las carpetas', () => {
  const carpetas = Array.from({ length: 8 }, (_, i) => ({
    id: `k${i}`,
    name: `c${i}`,
    isDirectory: true,
  }));
  const ficheros = Array.from({ length: 10 }, (_, i) => ({
    id: `f${i}`,
    name: `f${i}.txt`,
    url: `https://drive.sytue.eu/drive/files/f${i}`,
  }));
  expect(opcionesDrive([...carpetas, ...ficheros])).toHaveLength(8);
});
