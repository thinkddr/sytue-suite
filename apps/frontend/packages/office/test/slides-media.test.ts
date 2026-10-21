/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de medios incrustados (audio/vídeo) de slides:
 * detección de tipo (mediaKindFor), saneado del origen en la frontera de
 * confianza (safeMediaSrc) y normalización del descriptor del comando 'media'
 * (sanitizeMediaSpec). Cubre la ruta de SEGURIDAD (esquemas peligrosos fuera).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { mediaKindFor, safeMediaSrc, sanitizeMediaSpec } from '../src/editor/slides/SlidesCanvas';

test('mediaKindFor: por MIME y por extensión, insensible a mayúsculas', () => {
  expect(mediaKindFor('video/mp4')).toBe('video');
  expect(mediaKindFor('audio/mpeg')).toBe('audio');
  expect(mediaKindFor('clip.mp4')).toBe('video');
  expect(mediaKindFor('song.MP3')).toBe('audio');
  expect(mediaKindFor('movie.ogv')).toBe('video');
  expect(mediaKindFor('track.ogg')).toBe('audio'); // .ogg = audio; .ogv = vídeo
});

test('mediaKindFor: recorta query/hash y no confunde el punto del dominio', () => {
  expect(mediaKindFor('https://cdn.test/a/b/clip.webm?token=1#t=3')).toBe('video');
  expect(mediaKindFor('https://youtube.com/watch')).toBeNull(); // el «.» es del dominio, no extensión
  expect(mediaKindFor('sinextension')).toBeNull();
  expect(mediaKindFor('')).toBeNull();
});

test('safeMediaSrc: admite dataURL del tipo y http(s); normaliza dominio suelto a https', () => {
  expect(safeMediaSrc('data:video/mp4;base64,AAAA')).toBe('data:video/mp4;base64,AAAA');
  expect(safeMediaSrc('data:audio/mpeg;base64,AAA')).toBe('data:audio/mpeg;base64,AAA');
  expect(safeMediaSrc('https://x.com/a.mp4')).toBe('https://x.com/a.mp4');
  expect(safeMediaSrc('  ejemplo.com/v.mp4  ')).toBe('https://ejemplo.com/v.mp4');
});

test('safeMediaSrc: rechaza esquemas peligrosos y dataURL de otro tipo', () => {
  expect(safeMediaSrc('javascript:alert(1)')).toBeNull();
  expect(safeMediaSrc('blob:https://x')).toBeNull();
  expect(safeMediaSrc('file:///etc/passwd')).toBeNull();
  expect(safeMediaSrc('data:text/html,<b>x</b>')).toBeNull(); // no es audio/vídeo
  expect(safeMediaSrc('')).toBeNull();
});

test('safeMediaSrc: el póster admite dataURL de imagen y rechaza el de vídeo', () => {
  const img = /^data:image\//i;
  expect(safeMediaSrc('data:image/png;base64,AA', img)).toBe('data:image/png;base64,AA');
  expect(safeMediaSrc('data:video/mp4;base64,AA', img)).toBeNull(); // otro esquema/tipo para un póster
  expect(safeMediaSrc('https://x.com/p.png', img)).toBe('https://x.com/p.png');
});

test('sanitizeMediaSpec: normaliza las banderas (controles por defecto, autoplay/bucle off)', () => {
  expect(sanitizeMediaSpec({ mediaType: 'video', src: 'https://x.com/a.mp4' })).toEqual({
    mediaType: 'video',
    src: 'https://x.com/a.mp4',
    controls: true,
    autoplay: false,
    loop: false,
  });
  expect(
    sanitizeMediaSpec({
      mediaType: 'audio',
      src: 'clip.mp3',
      controls: false,
      autoplay: true,
      loop: true,
    }),
  ).toEqual({
    mediaType: 'audio',
    src: 'https://clip.mp3',
    controls: false,
    autoplay: true,
    loop: true,
  });
});

test('sanitizeMediaSpec: descarta tipo/origen inválidos o no-objeto', () => {
  expect(sanitizeMediaSpec({ mediaType: 'image', src: 'https://x/a.mp4' })).toBeNull();
  expect(sanitizeMediaSpec({ mediaType: 'video' })).toBeNull(); // sin src
  expect(sanitizeMediaSpec({ mediaType: 'video', src: 'javascript:alert(1)' })).toBeNull();
  expect(sanitizeMediaSpec(null)).toBeNull();
  expect(sanitizeMediaSpec('cadena')).toBeNull();
  expect(sanitizeMediaSpec(42)).toBeNull();
});

test('sanitizeMediaSpec: conserva un póster de imagen y omite uno peligroso', () => {
  const ok = sanitizeMediaSpec({
    mediaType: 'video',
    src: 'https://x/a.mp4',
    poster: 'data:image/png;base64,AA',
  });
  expect(ok?.poster).toBe('data:image/png;base64,AA');

  const bad = sanitizeMediaSpec({
    mediaType: 'video',
    src: 'https://x/a.mp4',
    poster: 'javascript:x',
  });
  expect(bad).not.toBeNull();
  expect(bad && 'poster' in bad).toBe(false); // póster peligroso descartado, medio insertado igual
});
