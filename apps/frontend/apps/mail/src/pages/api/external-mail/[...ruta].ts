/* SPDX-License-Identifier: Apache-2.0 */
import { crearProxyDeServicio, resolveIntegrationsUrl, serverEnv } from '@sytue/auth/server';
import { bff } from '../../../lib/bff.js';

export const prerender = false;
export const ALL = crearProxyDeServicio({
  bff,
  destino: resolveIntegrationsUrl(serverEnv(import.meta.env)),
  prefijo: '/api/v1/external-mail/',
  denegar: (path) =>
    !/^\/api\/v1\/external-mail\/(?:accounts|connect|[0-9a-f-]{36}(?:\/(?:folders|messages|message|send|actions))?)$/.test(
      path,
    ),
});
