/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { inlineSeguro, respuestaDeContenido } from './respuesta-contenido';

test('vídeo y audio se sirven inline (el reproductor los pinta en la vista previa)', () => {
  expect(inlineSeguro('video/mp4')).toBe(true);
  expect(inlineSeguro('audio/mpeg')).toBe(true);
  expect(inlineSeguro('application/octet-stream')).toBe(false);
  expect(inlineSeguro('text/html')).toBe(false);
});

test('un 206 del servicio llega como 206 con su Content-Range', () => {
  const r = respuestaDeContenido(
    {
      status: 206,
      headers: new Headers({
        'content-type': 'video/mp4',
        'content-length': '4',
        'content-range': 'bytes 2-5/10',
        'accept-ranges': 'bytes',
      }),
    },
    { name: 'clip.mp4', inline: true },
  );
  expect(r.status).toBe(206);
  expect(r.headers['content-range']).toBe('bytes 2-5/10');
  expect(r.headers['content-length']).toBe('4');
  expect(r.headers['accept-ranges']).toBe('bytes');
  expect(r.headers['content-disposition']).toBe('inline; filename="clip.mp4"');
});

test('el HTML sigue bajando como texto plano aunque se pida inline', () => {
  const r = respuestaDeContenido(
    { status: 200, headers: new Headers({ 'content-type': 'text/html' }) },
    { name: 'x.html', inline: true },
  );
  expect(r.status).toBe(200);
  expect(r.headers['content-type']).toBe('text/plain; charset=utf-8');
});
