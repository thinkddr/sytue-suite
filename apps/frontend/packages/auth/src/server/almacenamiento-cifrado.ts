/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Plomería compartida del BFF para almacenamiento cifrado E2E de la suite:
 *   - `/api/cipher-blobs/[...ruta]` → proxy en streaming (`duplex: 'half'`) hacia
 *     `/api/v1/cipher/blobs/{product}/{id}[/content]` en Sytue Drive, validando
 *     producto, identificador Base32, tope estricto de 8 MiB y cookie de sesión.
 *   - `/api/matrix/storage-bootstrap` → `GET /api/v1/matrix/storage/bootstrap`
 *     hacia Sytue Rooms con la identidad de la sesión httpOnly.
 *
 * Ningún token ni clave de cifrado pasa por aquí en claro hacia terceros: el BFF
 * solo traduce la cookie de sesión en `Authorization: Bearer` hacia dentro.
 */
import type { APIRoute } from 'astro';
import {
  ChatClient,
  MAX_CIPHER_BLOB_BYTES,
  bearer,
  isCipherBlobId,
  isCipherStorageProduct,
  upstreamStatus,
  type CipherStorageProduct,
} from '@sytue/api-clients';
import type { BffDeApp } from './app-bff.js';
import { toBffContext } from './app-bff.js';
import type { Bff } from './bff.js';
import { vieneDeOtroSitio } from './guard.js';
import { rutaQueEscapa } from './proxy.js';

export interface OpcionesRutaBlobsCifrados {
  bff: Bff | BffDeApp;
  destinoDrive: string | (() => string);
  /** Si la superficie fija su propio producto, rechaza cualquier otro con 403. */
  productoFijo?: CipherStorageProduct;
  fetchImpl?: typeof fetch;
}

export interface OpcionesRutaMatrixStorageBootstrap {
  bff: Bff | BffDeApp;
  destinoRooms: string | (() => string);
  /** Producto de almacenamiento privado fijado por la superficie (opcional). */
  productoFijo?: CipherStorageProduct;
  fetchImpl?: typeof fetch;
}

const CABECERAS_JSON = {
  'content-type': 'application/json; charset=utf-8',
  'cache-control': 'no-store',
  'x-content-type-options': 'nosniff',
} as const;

function errorJson(code: string, status: number): Response {
  return new Response(JSON.stringify({ error: { code } }), {
    status,
    headers: CABECERAS_JSON,
  });
}

function resolverDestino(destino: string | (() => string)): string {
  return (typeof destino === 'function' ? destino() : destino).replace(/\/$/, '');
}

export function crearRutaBlobsCifrados(opts: OpcionesRutaBlobsCifrados): {
  GET: APIRoute;
  PUT: APIRoute;
  DELETE: APIRoute;
} {
  const fetchFn = opts.fetchImpl ?? globalThis.fetch.bind(globalThis);
  const bffInstancia = 'getSession' in opts.bff ? opts.bff : opts.bff.bff;

  const handler: APIRoute = async (ctx) => {
    if (vieneDeOtroSitio(ctx.request)) {
      return errorJson('cross_site_forbidden', 403);
    }
    const raw = ctx.params.ruta ?? '';
    if (!raw || rutaQueEscapa(raw)) {
      return errorJson('invalid_path', 400);
    }
    const partes = raw.split('/').filter(Boolean);
    if (partes.length !== 2 && !(partes.length === 3 && partes[2] === 'content')) {
      return errorJson('not_found', 404);
    }
    const [productoRaw, blobId] = partes;
    const esContenido = partes.length === 3;
    if (!isCipherStorageProduct(productoRaw) || !isCipherBlobId(blobId)) {
      return errorJson('invalid_blob_reference', 400);
    }
    if (opts.productoFijo && productoRaw !== opts.productoFijo) {
      return errorJson('forbidden_product', 403);
    }

    const metodo = ctx.request.method.toUpperCase();
    if (esContenido && metodo !== 'GET') {
      return errorJson('method_not_allowed', 405);
    }
    if (!esContenido && metodo !== 'GET' && metodo !== 'PUT' && metodo !== 'DELETE') {
      return errorJson('method_not_allowed', 405);
    }

    const cabecerasHaciaDentro: Record<string, string> = {};
    if (metodo === 'PUT') {
      const longitudRaw = ctx.request.headers.get('content-length')?.trim() ?? '';
      if (!/^\d+$/.test(longitudRaw)) {
        return errorJson('length_required', 411);
      }
      const longitud = Number(longitudRaw);
      if (!Number.isSafeInteger(longitud) || longitud < 0) {
        return errorJson('invalid_length', 400);
      }
      if (longitud > MAX_CIPHER_BLOB_BYTES) {
        return errorJson('payload_too_large', 413);
      }
      const tipo = (ctx.request.headers.get('content-type') ?? '').toLowerCase();
      if (!tipo.startsWith('application/octet-stream')) {
        return errorJson('unsupported_media_type', 415);
      }
      cabecerasHaciaDentro['content-type'] = 'application/octet-stream';
      cabecerasHaciaDentro['content-length'] = String(longitud);
    }

    const sesion = await bffInstancia.getSession(toBffContext(ctx));
    if (!sesion) {
      return errorJson('unauthenticated', 401);
    }
    cabecerasHaciaDentro.authorization = `Bearer ${sesion.tokens.accessToken}`;

    const rutaServicio = `/api/v1/cipher/blobs/${encodeURIComponent(productoRaw)}/${encodeURIComponent(blobId)}${esContenido ? '/content' : ''}`;
    let res: Response;
    try {
      res = await fetchFn(`${resolverDestino(opts.destinoDrive)}${rutaServicio}`, {
        method: metodo,
        headers: cabecerasHaciaDentro,
        body: metodo === 'PUT' ? ctx.request.body : undefined,
        signal: ctx.request.signal,
        // @ts-expect-error duplex es obligatorio al enviar un cuerpo en streaming
        duplex: 'half',
      });
    } catch {
      return errorJson('cipher_storage_unavailable', 502);
    }

    const salida = new Headers({
      'cache-control': 'no-store',
      'x-content-type-options': 'nosniff',
    });
    const contentType = res.headers.get('content-type');
    if (contentType) salida.set('content-type', contentType);
    const contentLength = res.headers.get('content-length');
    if (contentLength) salida.set('content-length', contentLength);
    return new Response(res.body, { status: res.status, headers: salida });
  };

  return { GET: handler, PUT: handler, DELETE: handler };
}

export function crearRutaMatrixStorageBootstrap(opts: OpcionesRutaMatrixStorageBootstrap): {
  GET: APIRoute;
} {
  const bffInstancia = 'getSession' in opts.bff ? opts.bff : opts.bff.bff;

  const GET: APIRoute = async (ctx) => {
    if (vieneDeOtroSitio(ctx.request)) {
      return errorJson('cross_site_forbidden', 403);
    }
    const url = new URL(ctx.request.url);
    const pedido = url.searchParams.get('product');
    if (opts.productoFijo && pedido !== null && pedido !== opts.productoFijo) {
      return errorJson('forbidden_product', 403);
    }
    const producto = opts.productoFijo ?? pedido;
    if (!isCipherStorageProduct(producto)) {
      return errorJson('invalid_product', 400);
    }
    const sesion = await bffInstancia.getSession(toBffContext(ctx));
    if (!sesion) {
      return errorJson('unauthenticated', 401);
    }
    try {
      const chat = new ChatClient({
        baseUrl: resolverDestino(opts.destinoRooms),
        auth: bearer(sesion.tokens.accessToken),
        fetchImpl: opts.fetchImpl,
      });
      const data = await chat.matrixStorageBootstrap(producto, ctx.request.signal);
      return new Response(JSON.stringify(data), { status: 200, headers: CABECERAS_JSON });
    } catch (error) {
      return errorJson('matrix_unavailable', upstreamStatus(error));
    }
  };

  return { GET };
}
