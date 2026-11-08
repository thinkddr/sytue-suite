/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Compartir en Rooms» desde el lector. Lo que importa no se pinta: que a la sala
 * vaya el ENLACE del correo y no su contenido, con el prefijo de la app (sin él es el
 * 404 de Traefik, lo que le pasó a Drive), y que el BFF deje pasar el intent —una
 * allowlist que no lo nombra contesta 400 y el botón solo diría «no se pudo»—.
 * Que el lector pida el intent desde mail y abra la pestaña en el clic se prueba
 * pulsando el botón, en `lector-pestana.test.tsx`. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { textoParaRooms } from './utils.js';

test('el mensaje propuesto es el asunto y el enlace canónico, con prefijo y org', () => {
  const texto = textoParaRooms('Presupuesto', 'a1b2', 'acme');
  const [asunto, enlace] = texto.split('\n');
  expect(asunto).toBe('Presupuesto');
  expect(enlace).toMatch(/^https:\/\/[^/]+\/mail\/acme\/messages\/a1b2$/);
});

test('el BFF de Mail deja pasar share-to-chat', async () => {
  const bff = await Bun.file(new URL('../pages/api/intent.ts', import.meta.url)).text();
  expect(bff).toMatch(/ALLOWED = new Set\(\[[^\]]*'share-to-chat'/);
});
