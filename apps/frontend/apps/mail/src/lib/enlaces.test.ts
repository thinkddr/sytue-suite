// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { correoDeDestino, enlazarTexto, type Tramo } from './enlaces.js';

// Los vectores COMPARTIDOS con Go (apps/nucleo/htmltext/testdata/enlaces.json, que
// recorre enlaces_test.go): el mismo correo enlaza lo mismo en la web y en Fyne.
const vectores = JSON.parse(
  readFileSync(
    new URL('../../../../../nucleo/htmltext/testdata/enlaces.json', import.meta.url),
    'utf8',
  ),
) as { casos: { nombre: string; texto: string; tramos: Tramo[] }[] };

test('links match the vectors shared with the native app', () => {
  expect(vectores.casos.length).toBeGreaterThanOrEqual(10);
  for (const { nombre, texto, tramos } of vectores.casos) {
    expect({ nombre, tramos: enlazarTexto(texto) }).toEqual({ nombre, tramos });
    expect(
      enlazarTexto(texto)
        .map((t) => t.texto)
        .join(''),
    ).toBe(texto);
  }
});

test('only http(s) and mailto destinations ever come out', () => {
  const texto = 'a javascript:alert(1) data:text/html,x https://ok.es www.ok.es b@ok.es vbscript:x';
  for (const { destino } of enlazarTexto(texto)) {
    if (destino) expect(destino).toMatch(/^(https?:\/\/|mailto:)/i);
  }
});

test('correoDeDestino tells a mail destination from a web one', () => {
  expect(correoDeDestino('mailto:ana@sytue.eu')).toBe('ana@sytue.eu');
  expect(correoDeDestino('MAILTO:ana@sytue.eu')).toBe('ana@sytue.eu');
  expect(correoDeDestino('https://sytue.eu')).toBeNull();
});
