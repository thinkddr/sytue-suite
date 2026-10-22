/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { esTexto } from './nodes.js';

test('el texto se reconoce con y sin parámetros', () => {
  expect(esTexto('text/markdown')).toBe(true);
  expect(esTexto('text/plain; charset=utf-8')).toBe(true);
  expect(esTexto('APPLICATION/JSON')).toBe(true);
});

// Lo que hay que rechazar: sin extractor, un .docx llega como bytes comprimidos
// y el «resumen» saldría de basura binaria.
test('lo binario no pasa', () => {
  expect(esTexto('application/pdf')).toBe(false);
  expect(esTexto('application/vnd.openxmlformats-officedocument.wordprocessingml.document')).toBe(
    false,
  );
  expect(esTexto('')).toBe(false);
});
