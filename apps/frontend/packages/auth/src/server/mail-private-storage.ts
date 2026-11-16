/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { appDomains } from '@sytue/config';
import {
  encodeStorageScopeHeader,
  isMigrationEpoch,
  isStorageProduct,
  storageScopeHeader,
  type MatrixStorageProduct,
} from '@sytue/config/matrix';
import { sessionOwnerHeader } from '@sytue/config/session-owner';
import { serverEnv, type EnvLike } from '../config.js';
import type { Bff } from './bff.js';
import { crearProxyDeServicio } from './proxy.js';
import {
  privateStorageScope,
  privateStorageBody,
  readPrivateStorageBody,
  privateStorageFailure as fail,
} from './matrix-storage-scope.js';

interface PrivateRouteOptions {
  bff: Bff;
  env: EnvLike;
  fetchImpl?: typeof fetch;
}
function reply(response: Response): Response {
  response.headers.set('cache-control', 'no-store');
  response.headers.set('vary', 'Cookie');
  response.headers.set('x-content-type-options', 'nosniff');
  return response;
}
/** The existing Go APIs own source, control signatures and transient delivery.
 * This factory only fixes transport routes, framing and the authenticated owner. */
export function rutaMailPrivado(options: PrivateRouteOptions): APIRoute {
  const env = serverEnv(options.env);
  const proxy = crearProxyDeServicio({
    bff: options.bff,
    destino: env.SYTUE_MAIL_INTERNAL_URL ?? env.SYTUE_MAIL_API_URL ?? appDomains.mail,
    prefijo: '/api/v1/private-storage/',
    rechazarRedirecciones: true,
    ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
    revalidarSesion: true,
    prepararAutenticada: async (ctx, session) => {
      if (!ctx.request.headers.has(sessionOwnerHeader))
        return fail(409, 'private_storage_session_required');
      const scope = privateStorageScope(ctx.request, session, 'mail');
      if (scope instanceof Response) return scope;
      const parts = (ctx.params.ruta ?? '').split('/'),
        method = ctx.request.method;
      let postMaximum: number | undefined,
        stream = false;
      const extra: Record<string, string> = {};
      if (parts.length === 1 && parts[0] === 'migration') {
        if (method === 'POST') postMaximum = 256;
        else if (method !== 'GET') return fail(405, 'method_not_allowed');
      } else if (
        parts[0] === 'migration' &&
        isMigrationEpoch(parts[1]) &&
        parts.length === 3 &&
        parts[2] === 'rows'
      ) {
        if (method !== 'GET') return fail(405, 'method_not_allowed');
      } else if (
        parts[0] === 'migration' &&
        isMigrationEpoch(parts[1]) &&
        parts.length === 5 &&
        parts[2] === 'source'
      ) {
        if (method !== 'GET') return fail(405, 'method_not_allowed');
        if (
          !/^[A-Za-z0-9._:-]{1,256}$/u.test(parts[3] ?? '') ||
          !parts[4] ||
          new TextEncoder().encode(parts[4]).length > 8192
        )
          return fail(400, 'private_storage_source_invalid');
        extra['accept-encoding'] = 'identity';
      } else if (
        parts[0] === 'ingress' &&
        parts.length === 2 &&
        ['challenge', 'apply', 'pending', 'revalidate', 'body', 'enrollment'].includes(
          parts[1] ?? '',
        )
      ) {
        if (method !== 'POST') return fail(405, 'method_not_allowed');
        postMaximum = parts[1] === 'pending' ? 4096 : 256 << 10;
        extra['accept-encoding'] = 'identity';
      } else if (parts[0] === 'submissions' && parts.length === 1) {
        if (method !== 'POST') return fail(405, 'method_not_allowed');
        const submission = ctx.request.headers.get('x-sytue-mail-submission') ?? '';
        if (!/^[A-Za-z0-9_-]{1,32768}$/u.test(submission))
          return fail(400, 'private_mail_submission_invalid');
        extra['x-sytue-mail-submission'] = submission;
        postMaximum = 2 ** 47;
        stream = true;
      } else if (
        parts[0] === 'submissions' &&
        parts.length === 2 &&
        ['register', 'control'].includes(parts[1] ?? '')
      ) {
        if (method !== 'POST') return fail(405, 'method_not_allowed');
        postMaximum = parts[1] === 'register' ? 64 << 10 : 4096;
      } else if (
        parts[0] === 'submissions' &&
        parts.length === 3 &&
        isMigrationEpoch(parts[1]) &&
        isMigrationEpoch(parts[2])
      ) {
        if (method !== 'GET') return fail(405, 'method_not_allowed');
      } else return fail(400, 'private_storage_route_invalid');
      const query = ctx.url.searchParams;
      if (
        query.size &&
        !(
          parts[0] === 'migration' &&
          parts.length === 3 &&
          parts[2] === 'rows' &&
          query.size === 1 &&
          /^[A-Za-z0-9_-]{1,8192}$/u.test(query.get('after') ?? '')
        )
      )
        return fail(400, 'private_storage_source_invalid');
      if (postMaximum !== undefined) {
        const invalid = privateStorageBody(
          ctx.request,
          postMaximum,
          stream ? 'message/rfc822' : 'application/json',
          stream,
        );
        if (invalid) return invalid;
      } else if (ctx.request.body !== null) return fail(400, 'private_storage_body_invalid');
      const headers = { [storageScopeHeader]: encodeStorageScopeHeader(scope), ...extra };
      if (postMaximum !== undefined && !stream) {
        const body = await readPrivateStorageBody(ctx.request, postMaximum);
        if (body instanceof Response) return body;
        return { headers, body };
      }
      return headers;
    },
  });
  return async (ctx) => reply(await proxy(ctx));
}
/** Identity cutover is distinct from the SQL source snapshot epoch. */
export function rutaCorteIdentidadMatrixPrivada(
  options: PrivateRouteOptions & { product: MatrixStorageProduct },
): APIRoute {
  const env = serverEnv(options.env),
    product = options.product;
  if (!isStorageProduct(product)) throw new Error('unsupported_matrix_product');
  const proxy = crearProxyDeServicio({
    bff: options.bff,
    destino: env.SYTUE_ROOMS_INTERNAL_URL ?? env.SYTUE_ROOMS_API_URL ?? appDomains.rooms,
    prefijo: '/api/v1/matrix/storage/' + product + '/',
    rechazarRedirecciones: true,
    ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
    revalidarSesion: true,
    prepararAutenticada: async (ctx, session) => {
      if (!ctx.request.headers.has(sessionOwnerHeader))
        return fail(409, 'private_storage_session_required');
      const scope = privateStorageScope(ctx.request, session, product);
      if (scope instanceof Response) return scope;
      const path = ctx.params.ruta ?? '',
        method = ctx.request.method;
      if (!(
        (path === 'migration' && (method === 'GET' || method === 'POST')) ||
        (path === 'migration/complete' && method === 'POST')
      ))
        return fail(405, 'method_not_allowed');
      if (ctx.url.searchParams.size) return fail(400, 'private_storage_source_invalid');
      if (method === 'POST') {
        const invalid = privateStorageBody(ctx.request, 64 << 10);
        if (invalid) return invalid;
      } else if (ctx.request.body !== null) return fail(400, 'private_storage_body_invalid');
      const headers = { [storageScopeHeader]: encodeStorageScopeHeader(scope) };
      if (method === 'POST') {
        const body = await readPrivateStorageBody(ctx.request, 64 << 10);
        if (body instanceof Response) return body;
        return { headers, body };
      }
      return headers;
    },
  });
  return async (ctx) => {
    const raw = ctx.params.ruta ?? '';
    return reply(
      await proxy({
        ...ctx,
        params: { ...ctx.params, ruta: 'migration' + (raw ? '/' + raw : '') },
      }),
    );
  };
}
