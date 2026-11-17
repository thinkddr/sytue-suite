/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { appDomains } from '@sytue/config';
import {
  encodeStorageScopeHeader,
  storageScopeHeader,
  isStorageProduct,
  maxCipherBlockBytes,
  type MatrixStorageProduct,
} from '@sytue/config/matrix';
import { serverEnv, type EnvLike } from '../config.js';
import type { Bff } from './bff.js';
import { crearProxyDeServicio } from './proxy.js';
import { privateStorageScope, privateStorageFailure as fail } from './matrix-storage-scope.js';

/** Same-origin ciphertext stream. The only Sytue token belongs to the BFF session. */
export function rutaBlobsMatrixPrivados(options: {
  bff: Bff;
  env: EnvLike;
  product: MatrixStorageProduct;
  fetchImpl?: typeof fetch;
}): APIRoute {
  const product = options.product;
  if (!isStorageProduct(product)) throw new Error('unsupported_matrix_product');
  const env = serverEnv(options.env);
  const proxy = crearProxyDeServicio({
    destino: env.SYTUE_DRIVE_INTERNAL_URL ?? env.SYTUE_DRIVE_API_URL ?? appDomains.drive,
    bff: options.bff,
    prefijo: '/api/v1/cipher/blobs/',
    ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
    prepararAutenticada: (ctx, session) => {
      const path = ctx.params.ruta ?? '',
        match = new RegExp(`^${product}/[A-Z2-7]{26,64}(/content)?$`).exec(path);
      if (!match || ctx.url.search) return fail(400, 'cipher_blob_invalid');
      const method = ctx.request.method;
      if (method !== 'GET' && (match[1] || (method !== 'PUT' && method !== 'DELETE')))
        return fail(405, 'method_not_allowed');
      const scope = privateStorageScope(ctx.request, session, product);
      if (scope instanceof Response) return scope;
      if (method === 'PUT') {
        const length = ctx.request.headers.get('content-length') ?? '',
          size = Number(length);
        if (
          !/^(0|[1-9][0-9]*)$/.test(length) ||
          !Number.isSafeInteger(size) ||
          size > maxCipherBlockBytes ||
          ctx.request.headers.get('content-type') !== 'application/octet-stream' ||
          ctx.request.headers.has('content-encoding')
        )
          return fail(400, 'cipher_blob_invalid');
      } else if (ctx.request.body !== null) return fail(400, 'cipher_blob_invalid');
      return { [storageScopeHeader]: encodeStorageScopeHeader(scope) };
    },
  });
  return async (ctx) => {
    const response = await proxy(ctx);
    response.headers.set('cache-control', 'no-store');
    response.headers.set('vary', 'Cookie');
    return response;
  };
}
