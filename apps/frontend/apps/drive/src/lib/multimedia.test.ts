/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { tipoMultimedia } from './multimedia';

test('el content-type del servicio decide', () => {
  expect(tipoMultimedia('video/mp4', '')).toBe('video');
  expect(tipoMultimedia('audio/mpeg', 'bin')).toBe('audio');
  expect(tipoMultimedia('application/pdf', 'mp4')).toBeNull();
});

test('sin tipo concreto, la extensión hace de respaldo', () => {
  expect(tipoMultimedia('', 'webm')).toBe('video');
  expect(tipoMultimedia('application/octet-stream', 'MP3')).toBe('audio');
  // Formatos que el navegador no reproduce sin códecs extra: a descargar.
  expect(tipoMultimedia('', 'avi')).toBeNull();
  expect(tipoMultimedia('', 'mkv')).toBeNull();
});

// Drive no monta componentes en sus pruebas: se vigila el fuente del visor.
const fuente = readFileSync(new URL('../components/FileBrowser.tsx', import.meta.url), 'utf8');

test('la vista previa reproduce con controles, sin autoplay y por tramos', () => {
  expect(fuente).toMatch(/<video\s+src=\{mediaSrc\}\s+controls\s+preload="metadata"/);
  expect(fuente).toMatch(/<audio\s+src=\{mediaSrc\}\s+controls\s+preload="metadata"/);
  expect(fuente).not.toMatch(/<(video|audio)[^>]*autoPlay/);
  // Sin `offline=1`: el service worker no admite respuestas 206.
  expect(fuente).toMatch(/const mediaSrc =\s+inlineSrc \?\? `[^`]*disposition=inline`/);
});
