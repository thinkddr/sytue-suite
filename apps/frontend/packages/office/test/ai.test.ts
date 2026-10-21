/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las ayudas PURAS del asistente de IA:
 *  - buildAiRequest: mapa acción→directiva, prompt libre del chat, idioma de
 *    «traducir» y recorte del contexto a MAX_CONTEXT_CHARS.
 *  - parseAiResponse: extrae { text } o devuelve null si el cuerpo no es válido.
 *  - aiErrorKey: código del BFF → clave del catálogo (el texto lo pone la `t`).
 * Sin DOM (bun corre sin navegador). Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/ai.test.ts
 */
import { expect, test } from 'bun:test';
import { aiErrorKey, buildAiRequest, MAX_CONTEXT_CHARS, parseAiResponse } from '../src/editor/ai';
import { en } from '../src/i18n/messages/en';

test('buildAiRequest: una acción rápida resuelve su directiva y recorta el contexto', () => {
  const body = buildAiRequest({ action: 'resumir', context: '  hola mundo  ', kind: 'document' });
  expect(body.action).toBe('resumir');
  expect(body.instruction).toContain('Resume');
  expect(body.context).toBe('hola mundo'); // recortado por los extremos
  expect(body.kind).toBe('document');
});

test('buildAiRequest: el chat usa el prompt libre como directiva', () => {
  const body = buildAiRequest({
    action: 'chat',
    prompt: '  escribe un haiku sobre el mar  ',
    context: 'x',
    kind: 'document',
  });
  expect(body.instruction).toBe('escribe un haiku sobre el mar');
});

test('buildAiRequest: «traducir» compone el idioma destino (o inglés por defecto)', () => {
  const fr = buildAiRequest({
    action: 'traducir',
    prompt: 'francés',
    context: 'hi',
    kind: 'document',
  });
  expect(fr.instruction).toContain('francés');
  const def = buildAiRequest({ action: 'traducir', context: 'hi', kind: 'document' });
  expect(def.instruction).toContain('inglés');
});

test('buildAiRequest: recorta el contexto a MAX_CONTEXT_CHARS', () => {
  const long = 'a'.repeat(MAX_CONTEXT_CHARS + 500);
  const body = buildAiRequest({ action: 'resumir', context: long, kind: 'presentation' });
  expect(body.context.length).toBe(MAX_CONTEXT_CHARS);
});

test('buildAiRequest: contexto ausente → cadena vacía (no undefined)', () => {
  const body = buildAiRequest({ action: 'explicar', kind: 'spreadsheet' });
  expect(body.context).toBe('');
});

test('parseAiResponse: extrae text (recortado) y da null si falta o está vacío', () => {
  expect(parseAiResponse({ text: '  resultado  ' })).toBe('resultado');
  expect(parseAiResponse({ text: '   ' })).toBeNull();
  expect(parseAiResponse({ text: 42 })).toBeNull();
  expect(parseAiResponse({})).toBeNull();
  expect(parseAiResponse(null)).toBeNull();
  expect(parseAiResponse('nope')).toBeNull();
});

test('aiErrorKey: códigos conocidos, genérico y claves existentes en el catálogo', () => {
  expect(aiErrorKey('ai_not_configured')).toBe('ai.error.notConfigured');
  expect(aiErrorKey('ai_timeout')).toBe('ai.error.timeout');
  expect(aiErrorKey('upstream_unreachable')).toBe(aiErrorKey('ai_failed'));
  expect(aiErrorKey(undefined)).toBe('ai.error.generic');
  expect(aiErrorKey('código_raro')).toBe('ai.error.generic');
  // Toda clave devuelta tiene que existir en el catálogo base: si alguien la
  // renombra en `en.ts` y no aquí, el usuario vería la clave cruda en pantalla.
  for (const code of [
    'ai_not_configured',
    'unauthenticated',
    'ai_timeout',
    'bad_request',
    'ai_failed',
    undefined,
  ]) {
    expect(en[aiErrorKey(code)]).toBeDefined();
  }
});
