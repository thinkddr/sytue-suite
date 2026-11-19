/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { appDomains } from '@sytue/config';
import {
  encodeStorageScopeHeader,
  isStorageProduct,
  storageScopeHeader,
  type MatrixStorageProduct,
} from '@sytue/config/matrix';
import {
  cifradoE2EDesactivado,
  resolveDriveServiceUrl,
  serverEnv,
  type EnvLike,
} from '../config.js';
import { sesionApi } from './app-bff.js';
import type { Bff } from './bff.js';
import { vieneDeOtroSitio } from './guard.js';
import { crearProxyDeServicio } from './proxy.js';
import {
  privateStorageBody,
  privateStorageFailure as fail,
  privateStorageScope,
} from './matrix-storage-scope.js';

const maxKeyBodyBytes = 16 << 10;
const maxRecordBodyBytes = (96 << 10) + 2048;
const recordIdPath = /^records\/\$pqe_[A-Z2-7]{26,64}$/;

/**
 * Puerta BFF compartida para la bóveda post-cuántica directa (ADR 0086).
 *
 * - `GET bootstrap`: metadatos de sesión + origen de Sytue ID (sin llamar a Synapse ni a Rooms).
 * - `GET/PUT keys`: envoltorios públicos PQ (`age-MLKEM768-X25519-v1`) en `services/drive`.
 * - `GET/POST records` y `GET records/$pqe_...`: registros cifrados PQ en `services/drive`.
 */
export function rutaBovedaPq(options: {
  bff: Bff;
  env: EnvLike;
  product: MatrixStorageProduct;
  fetchImpl?: typeof fetch;
}): APIRoute {
  const product = options.product;
  if (!isStorageProduct(product)) throw new Error('unsupported_matrix_product');
  const sessionFor = sesionApi(options.bff);
  const env = serverEnv(options.env);
  const idOrigin = new URL(env.SYTUE_OIDC_ISSUER ?? appDomains.id).origin;
  const headers = { 'cache-control': 'no-store', vary: 'Cookie' };
  const proxy = crearProxyDeServicio({
    destino: resolveDriveServiceUrl(env),
    bff: options.bff,
    prefijo: `/api/v1/pq-vault/${product}/`,
    ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
    prepararAutenticada: (ctx, session) => {
      const path = ctx.params.ruta ?? '';
      const method = ctx.request.method;
      const isKeys = path === 'keys';
      const isRecords = path === 'records';
      const isRecord = recordIdPath.test(path);
      if (!isKeys && !isRecords && !isRecord) return fail(404, 'pq_vault_not_found');
      if (isKeys && method !== 'GET' && method !== 'PUT') return fail(405, 'method_not_allowed');
      if (isRecords && method !== 'GET' && method !== 'POST')
        return fail(405, 'method_not_allowed');
      if (isRecord && method !== 'GET') return fail(405, 'method_not_allowed');
      if ((isKeys || (isRecords && method === 'POST')) && ctx.url.search)
        return fail(400, 'pq_vault_invalid');
      const scope = privateStorageScope(ctx.request, session, product);
      if (scope instanceof Response) return scope;
      if (method === 'PUT' || method === 'POST') {
        const badBody = privateStorageBody(
          ctx.request,
          isKeys ? maxKeyBodyBytes : maxRecordBodyBytes,
        );
        if (badBody) return badBody;
      } else if (ctx.request.body !== null) {
        return fail(400, 'pq_vault_invalid');
      }
      return { [storageScopeHeader]: encodeStorageScopeHeader(scope) };
    },
  });
  return async (ctx) => {
    const path = ctx.params.ruta ?? '';
    if (path === 'bootstrap') {
      if (ctx.request.method !== 'GET') return fail(405, 'method_not_allowed');
      const origin = ctx.request.headers.get('origin');
      if (
        vieneDeOtroSitio(ctx.request) ||
        (origin !== null && origin !== new URL(ctx.request.url).origin)
      )
        return fail(403, 'cross_site');
      if (ctx.url.search) return fail(400, 'pq_vault_invalid');
      const session = await sessionFor(ctx);
      if (!session) return Response.json({ error: 'unauthenticated' }, { status: 401, headers });
      if (!session.user.tenant)
        return Response.json({ error: 'organisation_required' }, { status: 409, headers });
      if (cifradoE2EDesactivado(env))
        return Response.json({ cifrado: 'desactivado' }, { status: 200, headers });
      return Response.json(
        {
          actor: session.user.id,
          tenant: session.user.tenant,
          product,
          id_origin: idOrigin,
          encryption: 'age-MLKEM768-X25519-v1',
        },
        { status: 200, headers },
      );
    }
    const response = await proxy(ctx);
    response.headers.set('cache-control', 'no-store');
    response.headers.set('vary', 'Cookie');
    return response;
  };
}
