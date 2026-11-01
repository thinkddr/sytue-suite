/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import { MAX_CIPHER_BLOB_BYTES } from '@sytue/api-clients';
import type { Bff } from './bff.js';
import {
  crearRutaBlobsCifrados,
  crearRutaMatrixStorageBootstrap,
} from './almacenamiento-cifrado.js';

const BLOB_ID = 'A'.repeat(26);

function bffFalso(token: string | null): Bff {
  return {
    getSession: () => Promise.resolve(token ? { tokens: { accessToken: token } } : null),
  } as unknown as Bff;
}

function contexto(url: string, ruta: string | undefined, init: RequestInit = {}): APIContext {
  const request = new Request(url, init);
  return {
    params: ruta !== undefined ? { ruta } : {},
    request,
    url: new URL(request.url),
    cookies: {},
  } as APIContext;
}

test('crearRutaBlobsCifrados valida sesión, producto, Base32, límite de 8 MiB y reenvía en streaming', async () => {
  const llamadas: { url: string; method: string; auth: string | null; body: Uint8Array }[] = [];
  const fetchFalso = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input, init);
    const body = new Uint8Array(await req.arrayBuffer());
    llamadas.push({
      url: req.url,
      method: req.method,
      auth: req.headers.get('authorization'),
      body,
    });
    if (req.url.endsWith('/content')) {
      return new Response(new Uint8Array([9, 8, 7]), {
        status: 200,
        headers: { 'content-type': 'application/octet-stream', 'content-length': '3' },
      });
    }
    return new Response(
      JSON.stringify({
        id: BLOB_ID,
        product: 'drive',
        size_bytes: body.byteLength,
        sha256: 'a'.repeat(64),
      }),
      { status: 201, headers: { 'content-type': 'application/json' } },
    );
  }) as typeof fetch;

  const sinSesion = crearRutaBlobsCifrados({
    bff: bffFalso(null),
    destinoDrive: 'http://drive:8082',
    productoFijo: 'drive',
    fetchImpl: fetchFalso,
  });
  expect(
    (
      await sinSesion.GET(
        contexto('https://drive.sytue.test/api/cipher-blobs/drive/' + BLOB_ID, `drive/${BLOB_ID}`),
      )
    ).status,
  ).toBe(401);
  expect(llamadas).toHaveLength(0);

  const ruta = crearRutaBlobsCifrados({
    bff: bffFalso('jwt-secreto'),
    destinoDrive: 'http://drive:8082',
    productoFijo: 'drive',
    fetchImpl: fetchFalso,
  });

  // Rechaza petición cross-site (FE-02), escape de ruta, ID inválido, otro producto y > 8 MiB.
  expect(
    (
      await ruta.GET(
        contexto('https://drive.sytue.test/api/cipher-blobs/drive/' + BLOB_ID, `drive/${BLOB_ID}`, {
          headers: { 'sec-fetch-site': 'cross-site' },
        }),
      )
    ).status,
  ).toBe(403);
  expect(
    (await ruta.GET(contexto('https://drive.sytue.test/api/cipher-blobs', `drive/../internal`)))
      .status,
  ).toBe(400);
  expect(
    (await ruta.GET(contexto('https://drive.sytue.test/api/cipher-blobs', `drive/minusculas123`)))
      .status,
  ).toBe(400);
  expect(
    (await ruta.GET(contexto('https://drive.sytue.test/api/cipher-blobs', `mail/${BLOB_ID}`)))
      .status,
  ).toBe(403);
  expect(
    (
      await ruta.PUT(
        contexto('https://drive.sytue.test/api/cipher-blobs', `drive/${BLOB_ID}`, {
          method: 'PUT',
          headers: {
            'content-type': 'application/octet-stream',
            'content-length': String(MAX_CIPHER_BLOB_BYTES + 1),
          },
          body: new Uint8Array([1]),
        }),
      )
    ).status,
  ).toBe(413);
  expect(llamadas).toHaveLength(0);

  // PUT válido pasa el Bearer y devuelve no-store + nosniff.
  const putRes = await ruta.PUT(
    contexto('https://drive.sytue.test/api/cipher-blobs', `drive/${BLOB_ID}`, {
      method: 'PUT',
      headers: {
        'content-type': 'application/octet-stream',
        'content-length': '4',
      },
      body: new Uint8Array([1, 2, 3, 4]),
    }),
  );
  expect(putRes.status).toBe(201);
  expect(putRes.headers.get('cache-control')).toBe('no-store');
  expect(putRes.headers.get('x-content-type-options')).toBe('nosniff');
  expect(llamadas[0]?.url).toBe(`http://drive:8082/api/v1/cipher/blobs/drive/${BLOB_ID}`);
  expect(llamadas[0]?.auth).toBe('Bearer jwt-secreto');
  expect([...(llamadas[0]?.body ?? [])]).toEqual([1, 2, 3, 4]);

  // GET /content descarga el ciphertext opaco.
  const getRes = await ruta.GET(
    contexto('https://drive.sytue.test/api/cipher-blobs', `drive/${BLOB_ID}/content`),
  );
  expect(getRes.status).toBe(200);
  expect(new Uint8Array(await getRes.arrayBuffer())).toEqual(new Uint8Array([9, 8, 7]));
});

test('crearRutaMatrixStorageBootstrap fija el producto de la superficie y pasa solo el Bearer interno', async () => {
  let llamadaUrl = '';
  let llamadaAuth = '';
  const fetchFalso = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input, init);
    llamadaUrl = req.url;
    llamadaAuth = req.headers.get('authorization') ?? '';
    return new Response(
      JSON.stringify({
        actor: 'sub-1',
        tenant: 'acme',
        homeserver_url: 'https://matrix.sytue.eu',
        sso_provider_id: 'oidc-sytue',
        matrix_user_id: '@sytue_sub-1:matrix.sytue.eu',
        controller_user_id: '@rooms_controller:matrix.sytue.eu',
        channels: [],
        encryption: 'm.megolm.v1.aes-sha2',
        storage_product: 'drive',
        storage_room_id: '!drive:matrix.sytue.eu',
        storage_room_ids: ['!drive:matrix.sytue.eu'],
      }),
      { status: 200, headers: { 'content-type': 'application/json' } },
    );
  }) as typeof fetch;

  const ruta = crearRutaMatrixStorageBootstrap({
    bff: bffFalso('jwt-drive'),
    destinoRooms: 'http://rooms:8085',
    productoFijo: 'drive',
    fetchImpl: fetchFalso,
  });

  // Un producto distinto al de la superficie se rechaza con 403 sin llamar a Rooms.
  expect(
    (
      await ruta.GET(
        contexto('https://drive.sytue.test/api/matrix/storage-bootstrap?product=mail', undefined),
      )
    ).status,
  ).toBe(403);
  expect(llamadaUrl).toBe('');

  const res = await ruta.GET(
    contexto('https://drive.sytue.test/api/matrix/storage-bootstrap', undefined),
  );
  expect(res.status).toBe(200);
  expect(res.headers.get('cache-control')).toBe('no-store');
  expect(llamadaUrl).toBe('http://rooms:8085/api/v1/matrix/storage/bootstrap?product=drive');
  expect(llamadaAuth).toBe('Bearer jwt-drive');
});
