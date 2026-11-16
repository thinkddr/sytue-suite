/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { appDomains } from '@sytue/config';
import {
  encodeStorageScopeHeader,
  isStorageProduct,
  isMatrixRoomId,
  storageScopeHeader,
  type MatrixStorageProduct,
} from '@sytue/config/matrix';
import { serverEnv, type EnvLike } from '../config.js';
import type { Bff } from './bff.js';
import { crearProxyDeServicio } from './proxy.js';
import { privateStorageScope, privateStorageFailure as fail } from './matrix-storage-scope.js';

/** Public control transcripts only. The common proxy owns session/CSRF checks;
 * the authenticated backend owns nonces, signatures and durable generations. */
interface MatrixControlOptions {
  bff: Bff;
  env: EnvLike;
  product: MatrixStorageProduct;
  fetchImpl?: typeof fetch;
}

export function rutaAutoridadMatrixPrivada(options: MatrixControlOptions): APIRoute {
  return rutaControlMatrixPrivado(options, 'authority');
}

/** The common proxy retains session/generation/CSRF checks for every product.
 * Only fixed public admission operations are exposed, never an SDK token. */
export function rutaPublicacionesMatrixPrivadas(options: MatrixControlOptions): APIRoute {
  return rutaControlMatrixPrivado(options, 'publications');
}

function rutaControlMatrixPrivado(
  options: MatrixControlOptions,
  area: 'authority' | 'publications',
): APIRoute {
  if (!isStorageProduct(options.product)) throw new Error('unsupported_matrix_product');
  const product = options.product,
    env = serverEnv(options.env);
  const proxy = crearProxyDeServicio({
    destino: env.SYTUE_ROOMS_INTERNAL_URL ?? env.SYTUE_ROOMS_API_URL ?? appDomains.rooms,
    bff: options.bff,
    rechazarRedirecciones: true,
    prefijo: '/api/v1/matrix/storage/' + product + '/',
    ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
    prepararAutenticada: (ctx, session) => {
      const method = ctx.request.method,
        path = ctx.params.ruta ?? '';
      const validMethod =
        area === 'authority'
          ? (path === 'authority' && method === 'GET') ||
            ((path === 'authority/challenge' || path === 'authority/confirm') && method === 'POST')
          : ((path === 'publications' || path === 'publications/recovery') && method === 'GET') ||
            ((path === 'publications/reserve' || path === 'publications/cancel') &&
              method === 'POST');
      if (!validMethod) return fail(405, 'method_not_allowed');
      const origin = ctx.request.headers.get('origin');
      if (origin !== null && origin !== ctx.url.origin) return fail(403, 'cross_site');
      const query = ctx.url.searchParams;
      if (area === 'publications' && method === 'GET') {
        if (
          query.size !== 2 ||
          query.getAll('room_id').length !== 1 ||
          query.getAll('transaction_id').length !== 1 ||
          !isMatrixRoomId(query.get('room_id') ?? '') ||
          !/^[\x21-\x7e]{1,255}$/u.test(query.get('transaction_id') ?? '')
        )
          return fail(400, 'storage_publication_invalid');
      } else if (query.size !== 0) return fail(400, 'storage_control_invalid');
      const scope = privateStorageScope(ctx.request, session, product);
      if (scope instanceof Response) return scope;
      if (method === 'POST') {
        const length = ctx.request.headers.get('content-length') ?? '';
        if (
          !/^[1-9][0-9]{0,4}$/.test(length) ||
          Number(length) > (area === 'authority' ? 65536 : 16384) ||
          ctx.request.headers.get('content-type') !== 'application/json' ||
          ctx.request.headers.has('content-encoding')
        )
          return fail(400, 'storage_authority_invalid');
      } else if (ctx.request.body !== null) return fail(400, 'storage_authority_invalid');
      return { [storageScopeHeader]: encodeStorageScopeHeader(scope) };
    },
  });
  return async (ctx) => {
    const raw = ctx.params.ruta ?? '';
    const response = await proxy({
      ...ctx,
      params: { ...ctx.params, ruta: area + (raw ? '/' + raw : '') },
    });
    response.headers.set('cache-control', 'no-store');
    response.headers.set('vary', 'Cookie');
    return response;
  };
}
