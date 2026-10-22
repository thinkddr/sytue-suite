/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del compositor RFC 5322 del BFF (camino CCO): la copia oculta NUNCA
 * aparece en las cabeceras del mensaje, pero SÍ en el sobre (envelope) para que
 * se le entregue. Cubre además el encoded-word RFC 2047 del asunto y el
 * multipart/mixed con adjunto. Ejecutar: cd apps/frontend/apps/mail && bun test
 */
import { expect, test } from 'bun:test';
import { buildRfc5322, envelopeRecipients } from '../src/lib/rfc5322.js';

const from = { name: 'Ada Lovelace', email: 'ada@sytue.eu' };
const to = [{ email: 'bob@sytue.eu' }];
const cc = [{ name: 'Cae', email: 'cae@sytue.eu' }];
const bcc = [{ email: 'oculto@sytue.eu' }, { email: 'BOB@sytue.eu' }];

test('CCO nunca viaja en las cabeceras del mensaje', () => {
  const raw = buildRfc5322({ from, to, cc, subject: 'Hola', text: 'cuerpo' });
  const headers = raw.split('\r\n\r\n')[0]!;
  expect(headers).toContain('To: bob@sytue.eu');
  expect(headers).toContain('Cc: Cae <cae@sytue.eu>');
  expect(headers.toLowerCase()).not.toContain('bcc:');
  expect(raw).not.toContain('oculto@sytue.eu');
});

test('el sobre (envelope) incluye To+Cc+Bcc sin duplicar', () => {
  const rcpts = envelopeRecipients(to, cc, bcc);
  expect(rcpts).toContain('bob@sytue.eu');
  expect(rcpts).toContain('cae@sytue.eu');
  expect(rcpts).toContain('oculto@sytue.eu');
  // BOB@sytue.eu es el mismo destinatario que bob@sytue.eu (dedup case-insensitive).
  expect(rcpts.filter((r) => r.toLowerCase() === 'bob@sytue.eu')).toHaveLength(1);
});

test('cuerpo HTML => Content-Type text/html', () => {
  const raw = buildRfc5322({ from, to, subject: 'x', html: '<p>hola</p>' });
  expect(raw).toContain('Content-Type: text/html; charset=utf-8');
  expect(raw).toContain('<p>hola</p>');
});

test('asunto no-ASCII => encoded-word RFC 2047', () => {
  const raw = buildRfc5322({ from, to, subject: 'Pagó pendiente ñ', text: 'x' });
  expect(raw).toContain('Subject: =?utf-8?B?');
});

test('adjunto => multipart/mixed base64', () => {
  const raw = buildRfc5322({
    from,
    to,
    subject: 'con adjunto',
    text: 'cuerpo',
    attachments: [{ name: 'a.txt', type: 'text/plain', content: new Uint8Array([104, 105]) }],
  });
  expect(raw).toContain('Content-Type: multipart/mixed; boundary=');
  expect(raw).toContain('Content-Disposition: attachment; filename="a.txt"');
  expect(raw).toContain('Content-Transfer-Encoding: base64');
});
