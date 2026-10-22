// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// El salto a un mensaje que vive DENTRO de un hilo.
//
// El backend ancla la ventana en la raíz —el feed del canal solo pinta raíces— y
// devuelve `thread_root_id` para que el cliente sepa qué hilo abrir. Si ese campo
// se pierde por el camino, el salto deja al usuario mirando la raíz sin ver el
// mensaje que pidió: no falla nada, simplemente no aparece lo que buscaba, que es
// justo el fallo que nadie reporta como bug.
import { afterEach, expect, test } from 'bun:test';

import { chatClient } from './chat.js';

const fetchReal = globalThis.fetch;

function respondeCon(cuerpo: unknown) {
  // `as unknown as` y no `as typeof fetch` a secas: un doble que ignora sus
  // argumentos no solapa con la firma de `fetch`, y TS lo rechaza —con razón—
  // como conversión probablemente equivocada. Aquí es deliberado: el doble no
  // mira lo que se le pide, solo responde.
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as unknown as typeof fetch;
}

afterEach(() => {
  globalThis.fetch = fetchReal;
});

const UN_MENSAJE = [
  { id: 'raiz', channel_id: 'c', body: 'hola', created_at: '2026-08-17T10:00:00Z' },
];

test('messagesAround: propaga la raíz del hilo cuando el objetivo era una respuesta', async () => {
  respondeCon({ messages: UN_MENSAJE, thread_root_id: 'raiz' });

  const res = await chatClient({ accessToken: 't', userId: 'u' }).messagesAround(
    'c',
    'respuesta-1',
  );

  expect(res.threadRootId).toBe('raiz');
  expect(res.messages.length).toBe(1);
});

test('messagesAround: no inventa hilo cuando el objetivo ya era un mensaje raíz', async () => {
  respondeCon({ messages: UN_MENSAJE });

  const res = await chatClient({ accessToken: 't', userId: 'u' }).messagesAround('c', 'raiz');

  expect(res.threadRootId).toBeUndefined();
  expect(res.messages.length).toBe(1);
});
