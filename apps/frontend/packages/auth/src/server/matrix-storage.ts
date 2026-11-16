/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { appDomains } from '@sytue/config';
import { isStorageProduct, type MatrixStorageProduct } from '@sytue/config/matrix';
import { ChatClient, bearer, upstreamStatus } from '@sytue/api-clients';
import { serverEnv, type EnvLike } from '../config.js';
import { sesionApi } from './app-bff.js';
import type { Bff } from './bff.js';

/** BFF metadata only. Matrix credentials, content and recovery keys never belong on this route. */
export function rutaBootstrapMatrixPrivado(options: {
  bff: Bff;
  env: EnvLike;
  product: MatrixStorageProduct;
  fetchImpl?: typeof fetch;
}): APIRoute {
  if (!isStorageProduct(options.product)) throw new Error('unsupported_matrix_product');
  const product = options.product,
    sessionFor = sesionApi(options.bff);
  const env = serverEnv(options.env);
  const baseUrl = env.SYTUE_ROOMS_INTERNAL_URL ?? env.SYTUE_ROOMS_API_URL ?? appDomains.rooms;
  const headers = { 'cache-control': 'no-store', vary: 'Cookie' };
  return async (context) => {
    const session = await sessionFor(context);
    if (!session) return Response.json({ error: 'unauthenticated' }, { status: 401, headers });
    const actor = session.user.id,
      tenant = session.user.tenant;
    if (!tenant) return Response.json({ error: 'organisation_required' }, { status: 409, headers });
    const versions = new URL(context.request.url).searchParams.getAll('identity_version');
    if (
      versions.length > 1 ||
      (versions.length === 1 && versions[0] !== '1' && versions[0] !== '2')
    ) {
      return Response.json({ error: 'matrix_identity_version' }, { status: 400, headers });
    }
    const identityVersion = versions[0] === '2' ? 2 : 1;
    try {
      context.request.signal.throwIfAborted();
      const client = new ChatClient({
        baseUrl,
        auth: bearer(session.accessToken),
        ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
      });
      const data = await client.matrixStorageBootstrap(
        product,
        context.request.signal,
        identityVersion,
      );
      context.request.signal.throwIfAborted();
      if (
        data.actor !== actor ||
        data.tenant !== tenant ||
        data.storage_product !== product ||
        (data.storage_identity_version === undefined || data.storage_identity_version === 0
          ? 1
          : data.storage_identity_version) !== identityVersion ||
        !Array.isArray(data.channels) ||
        data.channels.length !== 0 ||
        data.draft_room_id ||
        (data.draft_room_ids?.length ?? 0) !== 0 ||
        !Array.isArray(data.storage_room_ids) ||
        data.storage_room_ids.length === 0 ||
        data.storage_room_ids.length > 100 ||
        data.storage_room_ids.some((room) => typeof room !== 'string') ||
        typeof data.storage_room_id !== 'string' ||
        !data.storage_room_ids.includes(data.storage_room_id) ||
        typeof data.homeserver_url !== 'string' ||
        typeof data.sso_provider_id !== 'string' ||
        typeof data.matrix_user_id !== 'string' ||
        typeof data.controller_user_id !== 'string' ||
        data.encryption !== 'm.megolm.v1.aes-sha2'
      )
        return Response.json({ error: 'matrix_identity' }, { status: 502, headers });
      // Explicit DTO: a misrouted upstream response cannot smuggle credentials
      // or arbitrary properties into browser bootstrap JSON.
      return Response.json(
        {
          actor,
          tenant,
          homeserver_url: data.homeserver_url,
          sso_provider_id: data.sso_provider_id,
          matrix_user_id: data.matrix_user_id,
          controller_user_id: data.controller_user_id,
          encryption: data.encryption,
          channels: [],
          storage_product: product,
          storage_identity_version: identityVersion,
          storage_room_id: data.storage_room_id,
          storage_room_ids: [...data.storage_room_ids],
        },
        { headers },
      );
    } catch (error) {
      return Response.json(
        { error: 'matrix_unavailable' },
        { status: upstreamStatus(error), headers },
      );
    }
  };
}
