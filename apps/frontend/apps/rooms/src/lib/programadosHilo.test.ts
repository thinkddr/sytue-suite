// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Una respuesta PROGRAMADA conserva su hilo y su «También enviar al canal» al listarse:
// «Enviar ahora» (ChatApp) la publica con ellos, igual que el servicio al vencer. Sin el
// campo, salía como respuesta solo del hilo (o como mensaje suelto del canal).
import { afterEach, expect, test } from 'bun:test';

import { chatClient } from './chat.js';

const fetchReal = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = fetchReal;
});

test('listScheduled: una respuesta programada trae su raíz y «también al canal»', async () => {
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({
        scheduled: [
          {
            id: 'p1',
            channel_id: 'c',
            parent_id: 'raiz',
            author_sub: 'u',
            body: 'luego',
            send_at: '2026-11-20T09:00:00Z',
            created_at: '2026-11-16T09:00:00Z',
            also_in_channel: true,
          },
          {
            id: 'p2',
            channel_id: 'c',
            author_sub: 'u',
            body: 'raíz',
            send_at: '2026-11-20T09:00:00Z',
            created_at: '2026-11-16T09:00:00Z',
          },
        ],
      }),
      { status: 200, headers: { 'content-type': 'application/json' } },
    )) as unknown as typeof fetch;

  const [respuesta, raiz] = await chatClient({ accessToken: 't', userId: 'u' }).listScheduled();

  expect(respuesta?.parentId).toBe('raiz');
  expect(respuesta?.alsoInChannel).toBe(true);
  expect(raiz?.parentId).toBeUndefined();
  expect(raiz?.alsoInChannel).toBeUndefined();
});
