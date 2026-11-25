/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que el BFF deja pasar de un fallo del servicio: el 4xx tal cual y, de los códigos,
 * solo los que la isla distingue (la política de contacto con gente de fuera). Lo demás
 * sigue siendo `chat_error`, y un 5xx o un fallo de red, 502.
 */
import { expect, test } from 'bun:test';
import { ChatHttpError, chatFail } from './chat.js';

async function cuerpo(r: Response): Promise<{ error: string }> {
  return (await r.json()) as { error: string };
}

test('el código de la política de contacto llega a la isla con su 403', async () => {
  const r = chatFail(new ChatHttpError(403, 'x', undefined, 'rooms_external_closed'));
  expect(r.status).toBe(403);
  expect((await cuerpo(r)).error).toBe('rooms_external_closed');
});

test('cualquier otro código se queda en el servidor', async () => {
  const r = chatFail(new ChatHttpError(403, 'x', undefined, 'blocked'));
  expect(r.status).toBe(403);
  expect((await cuerpo(r)).error).toBe('chat_error');
});

test('un 5xx o un error cualquiera es 502', async () => {
  expect(chatFail(new ChatHttpError(500, 'x')).status).toBe(502);
  expect(chatFail(new Error('red')).status).toBe(502);
});
