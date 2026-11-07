/* SPDX-License-Identifier: Apache-2.0 */
import { crearProxyDeServicio, resolveIntegrationsUrl, serverEnv } from '@sytue/auth/server';
import { bff } from '../../../lib/bff.js';
import { rutaCorreoExternoDenegada } from '../../../lib/rutasCorreoExterno.js';

export const prerender = false;
export const ALL = crearProxyDeServicio({
  bff,
  destino: resolveIntegrationsUrl(serverEnv(import.meta.env)),
  prefijo: '/api/v1/external-mail/',
  denegar: rutaCorreoExternoDenegada,
});
