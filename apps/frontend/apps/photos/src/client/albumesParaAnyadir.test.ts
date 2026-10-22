/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { afterEach, expect, test } from 'bun:test';
import { albumesParaAnyadir, anyadirAlAlbum, sinEspacio } from './photosApi.js';

const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

function servicio(responder: (ruta: string) => Response): void {
  globalThis.fetch = (async (url: string) => responder(String(url))) as typeof fetch;
}

test('a «Añadir a un álbum…» van los míos y los compartidos PARA EDITAR, no los de solo ver', async () => {
  servicio((ruta) =>
    ruta.endsWith('/api/v1/albums')
      ? Response.json({ items: [{ id: 'mio', title: 'Mío', coverAssetId: null, count: 1 }] })
      : Response.json({
          items: [
            {
              id: 'ed',
              title: 'De Ada',
              permission: 'write',
              sharedBy: 'ada',
              sharedByName: 'Ada',
            },
            { id: 'ver', title: 'Solo ver', permission: 'read', sharedBy: 'ana' },
          ],
        }),
  );
  const lista = await albumesParaAnyadir();
  expect(lista.map((a) => [a.id, a.compartidoPor])).toEqual([
    ['mio', undefined],
    ['ed', 'Ada'],
  ]);
});

test('sin espacio en el Drive de la dueña (507) se distingue de cualquier otro fallo', async () => {
  servicio(() => new Response('{"error":{"code":"quota_exceeded"}}', { status: 507 }));
  const e = await anyadirAlAlbum('ed', ['f1']).catch((x: unknown) => x);
  expect(sinEspacio(e)).toBe(true);
  servicio(() => new Response('{}', { status: 500 }));
  expect(sinEspacio(await anyadirAlAlbum('ed', ['f1']).catch((x: unknown) => x))).toBe(false);
});
