/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { afterEach, expect, test } from 'bun:test';
import { albumesParaAnyadir, anyadirAlAlbum, ponerPortada, sinEspacio } from './photosApi.js';

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

test('elegir portada manda el título de ahora y la foto, por PATCH al álbum', async () => {
  const vistos: { metodo: string; ruta: string; cuerpo: unknown }[] = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    vistos.push({
      metodo: init?.method ?? 'GET',
      ruta: String(url),
      cuerpo: JSON.parse(String(init?.body)),
    });
    return Response.json({ id: 'a 1', title: 'Verano', coverAssetId: 'f2', count: 3 });
  }) as typeof fetch;
  const al = await ponerPortada('a 1', 'Verano', 'f2');
  // La ruta lleva delante la base de la app (`/photos` según quién la fije antes).
  expect(vistos).toHaveLength(1);
  expect(vistos[0]!.ruta.endsWith('/api/v1/albums/a%201')).toBe(true);
  expect(vistos[0]).toMatchObject({
    metodo: 'PATCH',
    cuerpo: { title: 'Verano', coverAssetId: 'f2' },
  });
  expect(al.coverAssetId).toBe('f2');
});
