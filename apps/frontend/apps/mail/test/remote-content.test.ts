/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Detección de contenido remoto en el cuerpo de un correo. Es lo que decide si
 * el lector enseña la barra «imágenes bloqueadas»: la CSP del visor bloquea
 * igual, pero sin esta detección no habría forma de pedir que se muestren.
 * Ejecutar: cd apps/frontend/apps/mail && bun test
 */
import { expect, test } from 'bun:test';
import { hasRemoteContent } from '../src/components/utils.js';

test('detecta el píxel de rastreo clásico', () => {
  expect(hasRemoteContent('<img src="https://rastreo.example/abrir.gif?id=1" width="1">')).toBe(
    true,
  );
  expect(hasRemoteContent("<img src='http://rastreo.example/p.gif'>")).toBe(true);
  expect(hasRemoteContent('<img src = "https://rastreo.example/p.gif">')).toBe(true);
});

test('detecta las variantes que no son un <img src> directo', () => {
  expect(hasRemoteContent('<img srcset="https://cdn.example/a.png 2x">')).toBe(true);
  expect(hasRemoteContent('<td background="https://cdn.example/fondo.png">')).toBe(true);
  expect(hasRemoteContent('<video poster="https://cdn.example/p.jpg"></video>')).toBe(true);
  expect(hasRemoteContent('<div style="background-image:url(https://cdn.example/f.png)">')).toBe(
    true,
  );
  expect(hasRemoteContent("<style>body{background:url('https://cdn.example/f.png')}</style>")).toBe(
    true,
  );
});

test('no avisa cuando no hay nada que descargar de fuera', () => {
  expect(hasRemoteContent('<p>Hola, ¿cómo estás?</p>')).toBe(false);
  // Imagen incrustada en el propio mensaje: la CSP la deja pasar, no hay aviso.
  expect(hasRemoteContent('<img src="data:image/png;base64,iVBORw0KGgo=">')).toBe(false);
  expect(hasRemoteContent('<img src="cid:logo@example">')).toBe(false);
  // Un enlace no descarga nada hasta que se pulsa.
  expect(hasRemoteContent('<a href="https://example.com/oferta">Ver oferta</a>')).toBe(false);
});
