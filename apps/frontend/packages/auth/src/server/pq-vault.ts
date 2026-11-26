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
import { contextoDerivado } from './contexto-derivado.js';

const maxKeyBodyBytes = 16 << 10;
// Una clave pública híbrida y dos firmas ML-DSA-65, en base64: unos 14 KB.
const maxSigningKeyBodyBytes = 24 << 10;
const maxRecordBodyBytes = (96 << 10) + 2048;
const recordIdPath = /^records\/\$pqe_[A-Z2-7]{26,64}$/;
const copyIdQuery = /^\?id=[A-Za-z0-9_-]{22,64}$/;

/**
 * Puerta BFF compartida para la bóveda post-cuántica directa (ADR 0086).
 *
 * - `GET bootstrap`: metadatos de sesión + origen de Sytue ID (sin llamar a Synapse ni a Rooms).
 * - `GET/PUT keys`: envoltorios públicos PQ (`age-MLKEM768-X25519-v1`) en `services/drive`.
 * - `GET/POST records` y `GET records/$pqe_...`: registros cifrados PQ en `services/drive`.
 * - `DELETE copy?id=…`: cancelar la marca «copia en curso» que puso otro
 *   dispositivo (ADR 0086). La web no la pone: solo la ve (en `keys`) y la cancela.
 * - `GET/PUT signing-key`: la clave de firma de la bóveda, registrada en Sytue ID (ADR 0087).
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
      const isCopy = path === 'copy';
      if (!isKeys && !isRecords && !isRecord && !isCopy) return fail(404, 'pq_vault_not_found');
      if (isCopy && method !== 'DELETE') return fail(405, 'method_not_allowed');
      if (isCopy && !copyIdQuery.test(ctx.url.search)) return fail(400, 'pq_vault_invalid');
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
  // La clave de firma de la bóveda (ADR 0087): el historial y el registro viven en
  // Sytue ID, que los acota con el token de la persona. El BFF solo reenvía.
  const proxyClaveDeFirma = crearProxyDeServicio({
    destino: (env.SYTUE_OIDC_INTERNAL_BASE_URL ?? env.SYTUE_OIDC_ISSUER ?? appDomains.id).replace(
      /\/$/,
      '',
    ),
    bff: options.bff,
    prefijo: `/api/v1/me/vault-keys/${product}`,
    ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
    prepararAutenticada: (ctx) => {
      const method = ctx.request.method;
      if (method !== 'GET' && method !== 'PUT') return fail(405, 'method_not_allowed');
      if (ctx.url.search) return fail(400, 'pq_vault_invalid');
      if (method === 'PUT') {
        const badBody = privateStorageBody(ctx.request, maxSigningKeyBodyBytes);
        if (badBody) return badBody;
      } else if (ctx.request.body !== null) {
        return fail(400, 'pq_vault_invalid');
      }
      return {};
    },
  });
  return async (ctxRecibido) => {
    // Astro descodifica la ruta con `decodeURI`, que deja los reservados como están:
    // `records/%24pqe_…` llegaba así y no casaba con `recordIdPath` (404), y Drive se
    // quedaba en `prepare/record.invalid`. Un `%24` delante de `pqe_` es el `$` del
    // id; cualquier otra cosa sigue pasando por la misma validación estricta.
    const recibida = ctxRecibido.params.ruta ?? '';
    const path = recibida.replace(/^records\/%24(?=pqe_)/iu, 'records/$');
    const ctx =
      path === recibida
        ? ctxRecibido
        : contextoDerivado(ctxRecibido, { ...ctxRecibido.params, ruta: path });
    if (path === 'signing-key') {
      const origin = ctx.request.headers.get('origin');
      if (
        vieneDeOtroSitio(ctx.request) ||
        (origin !== null && origin !== new URL(ctx.request.url).origin)
      )
        return fail(403, 'cross_site');
      // El proxy compone la ruta con `ruta`: aquí va vacía, la fija el prefijo.
      const sub = contextoDerivado(ctx, { ...ctx.params, ruta: '' });
      const response = await proxyClaveDeFirma(sub);
      response.headers.set('cache-control', 'no-store');
      response.headers.set('vary', 'Cookie');
      return response;
    }
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
