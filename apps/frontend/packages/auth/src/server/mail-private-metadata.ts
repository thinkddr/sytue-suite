/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { appDomains } from '@sytue/config';
import {
  encodeStorageScopeHeader,
  isMigrationEpoch,
  storageScopeHeader,
} from '@sytue/config/matrix';
import { sessionOwnerHeader } from '@sytue/config/session-owner';
import { serverEnv, type EnvLike } from '../config.js';
import type { Bff } from './bff.js';
import { crearProxyDeServicio, type OpcionesProxyDeServicio } from './proxy.js';
import {
  privateStorageScope,
  privateStorageBody,
  privateStorageFailure as fail,
} from './matrix-storage-scope.js';

/** Public delivery addresses and opaque-byte counters only. No MIME/content route. */
export function rutaMetadataMailPrivado(options: {
  bff: Bff;
  env: EnvLike;
  fetchImpl?: typeof fetch;
}): APIRoute {
  const env = serverEnv(options.env);
  const prepare: NonNullable<OpcionesProxyDeServicio['prepararAutenticada']> = (ctx, session) => {
    if (!ctx.request.headers.has(sessionOwnerHeader))
      return fail(409, 'private_storage_session_required');
    const scope = privateStorageScope(ctx.request, session, 'mail');
    if (scope instanceof Response) return scope;
    const path = ctx.params.ruta ?? '',
      method = ctx.request.method;
    const alias = path === 'aliases',
      deletion = path.startsWith('aliases/') && isMigrationEpoch(path.slice('aliases/'.length));
    if (ctx.url.search) return fail(400, 'private_metadata_route_invalid');
    if (!(
      (alias && (method === 'GET' || method === 'POST')) ||
      (deletion && method === 'DELETE') ||
      ((path === 'quota' || path === 'usage/mail') && method === 'GET')
    ))
      return fail(405, 'method_not_allowed');
    if (alias && method === 'POST') {
      const invalid = privateStorageBody(ctx.request, 2048);
      if (invalid) return invalid;
    } else if (ctx.request.body !== null) return fail(400, 'private_storage_body_invalid');
    return { [storageScopeHeader]: encodeStorageScopeHeader(scope) };
  };
  const common = {
    bff: options.bff,
    rechazarRedirecciones: true,
    prepararAutenticada: prepare,
    ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
  };
  const mail = crearProxyDeServicio({
    ...common,
    destino: env.SYTUE_MAIL_INTERNAL_URL ?? env.SYTUE_MAIL_API_URL ?? appDomains.mail,
    prefijo: '/api/v1/',
  });
  const usage = crearProxyDeServicio({
    ...common,
    destino: env.SYTUE_DRIVE_INTERNAL_URL ?? env.SYTUE_DRIVE_API_URL ?? appDomains.drive,
    prefijo: '/api/v1/cipher/',
  });
  return async (ctx) => {
    const path = ctx.params.ruta ?? '';
    if (
      path !== 'usage' &&
      path !== 'quota' &&
      path !== 'aliases' &&
      !(path.startsWith('aliases/') && isMigrationEpoch(path.slice('aliases/'.length)))
    )
      return fail(400, 'private_metadata_route_invalid');
    const response =
      path === 'usage'
        ? await usage({ ...ctx, params: { ...ctx.params, ruta: 'usage/mail' } })
        : await mail(ctx);
    response.headers.set('cache-control', 'no-store');
    response.headers.set('vary', 'Cookie');
    response.headers.set('x-content-type-options', 'nosniff');
    return response;
  };
}
