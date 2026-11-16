// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { CoEdicion } from './coedicion.js';
import { parseCursorPos, parsePeerCursor } from './cursors.js';
import { longitudVisible } from './textohtml.js';
import type { CursorPos, PeerCursor } from '../types.js';

// Los vectores COMPARTIDOS con Go (apps/nucleo/coedicion/testdata/cursores.json, que
// recorren coedicion/cursor_test.go y api/docs_cursor_test.go): con el mismo HTML y los
// mismos offsets, la web y Fyne anclan el cursor a los mismos caracteres y leen igual las
// posiciones que llegan. Si divergen, el cursor de una persona sale en otro sitio en la
// pantalla de la otra.
type Off = { anchor: number; focus: number };
type Dots = { anchorDot: { a: string; c: number }; focusDot: { a: string; c: number } };
const vectores = JSON.parse(
  readFileSync(
    new URL('../../../../../../nucleo/coedicion/testdata/cursores.json', import.meta.url),
    'utf8',
  ),
) as {
  mapeo: (Off & { nombre: string; html: string; puntos: Dots | null })[];
  edicion: (Off & { nombre: string; html: string; nuevo: string; esperado: Off })[];
  posiciones: { nombre: string; crudo: unknown; esperado: CursorPos | null }[];
  pares: {
    nombre: string;
    crudo: unknown;
    esperado: PeerCursor | null;
  }[];
};

/** Quien ha escrito `html` de una vez: actor «ada», documento nuevo, a solas. */
const escrito = (html: string): CoEdicion => {
  const c = new CoEdicion('ada', [], '');
  c.opsLocales(html, 0, true);
  return c;
};

test('cursores: mismos offsets, mismos puntos, y de vuelta', () => {
  expect(vectores.mapeo.length).toBeGreaterThanOrEqual(8);
  for (const { nombre, html, anchor, focus, puntos } of vectores.mapeo) {
    const co = escrito(html);
    const largo = longitudVisible(html);
    expect({ nombre, puntos: co.puntosDeOffsets(largo, { anchor, focus }) }).toEqual({
      nombre,
      puntos,
    });
    if (puntos)
      expect({ nombre, off: co.offsetsDePuntos(largo, puntos) }).toEqual({
        nombre,
        off: { anchor, focus },
      });
  }
});

test('cursores: el anclado sigue a sus caracteres tras editar', () => {
  expect(vectores.edicion.length).toBeGreaterThanOrEqual(4);
  for (const { nombre, html, nuevo, anchor, focus, esperado } of vectores.edicion) {
    const co = escrito(html);
    const puntos = co.puntosDeOffsets(longitudVisible(html), { anchor, focus });
    co.opsLocales(nuevo, 100, true);
    expect({ nombre, off: puntos && co.offsetsDePuntos(longitudVisible(nuevo), puntos) }).toEqual({
      nombre,
      off: esperado,
    });
  }
});

test('cursores: las posiciones recibidas se leen y se descartan igual que en Fyne', () => {
  expect(vectores.posiciones.length).toBeGreaterThanOrEqual(10);
  for (const { nombre, crudo, esperado } of vectores.posiciones) {
    expect({ nombre, pos: parseCursorPos(crudo) }).toEqual({ nombre, pos: esperado });
  }
  for (const { nombre, crudo, esperado } of vectores.pares) {
    const p = parsePeerCursor(crudo);
    // Go guarda el token del color (`avatar-N`); la web lo da como su variable CSS.
    const leido = p && { ...p, color: p.color.replace(/^var\(--sytue-(avatar-\d)\)$/, '$1') };
    expect({ nombre, par: leido }).toEqual({ nombre, par: esperado });
  }
});
