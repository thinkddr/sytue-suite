/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Descargar la conversación» (el mismo texto que `ConversacionEnTexto` de Fyne) y
 * «Marcar todo como leído» de pocas en pocas.
 */
import { expect, test } from 'bun:test';
import {
  conversacionEnTexto,
  fechaHoraLocal,
  nombreDeConversacion,
} from './conversacionEnTexto.js';
import { enTandas } from './enTandas.js';

test('una línea por mensaje con su hora local y su autor; lo largo, sangrado', () => {
  const a = new Date(2026, 10, 25, 9, 5);
  const b = new Date(2026, 10, 25, 9, 7);
  const texto = conversacionEnTexto('general', [
    { author: { name: 'Ana' }, body: 'Hola', createdAt: a.toISOString() },
    { author: { name: 'Luis' }, body: 'Dos\nlíneas', createdAt: b.toISOString() },
  ]);
  expect(texto).toBe(
    `# general\n\n[${fechaHoraLocal(a)}] Ana: Hola\n[${fechaHoraLocal(b)}] Luis: Dos\n    líneas\n`,
  );
  expect(fechaHoraLocal(a)).toBe('2026-11-25 09:05');
});

test('el fichero se llama como el canal, sin lo que un sistema de ficheros rechaza', () => {
  expect(nombreDeConversacion('ventas/norte: Q4', new Date(2026, 10, 25))).toBe(
    'ventas norte Q4 2026-11-25.txt',
  );
  expect(nombreDeConversacion('///', new Date(2026, 10, 25))).toBe('rooms 2026-11-25.txt');
});

test('enTandas no pasa de N a la vez, hace todas y un fallo no para a las demás', async () => {
  let enVuelo = 0;
  let maximo = 0;
  const hechas: number[] = [];
  const bien = await enTandas([1, 2, 3, 4, 5, 6, 7, 8, 9, 10], 4, async (n) => {
    enVuelo++;
    maximo = Math.max(maximo, enVuelo);
    await new Promise((r) => setTimeout(r, 1));
    enVuelo--;
    if (n === 3) throw new Error('429');
    hechas.push(n);
  });
  expect(maximo).toBe(4);
  expect(bien).toBe(9);
  expect(hechas.sort((x, y) => x - y)).toEqual([1, 2, 4, 5, 6, 7, 8, 9, 10]);
  expect(await enTandas([], 4, async () => undefined)).toBe(0);
});
