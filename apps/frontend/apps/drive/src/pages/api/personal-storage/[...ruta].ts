/* SPDX-License-Identifier: Apache-2.0 */
// Nubes personales (Google Drive, OneDrive, Dropbox): las sirve el servicio de
// integraciones con la credencial de la propia persona.
import { crearProxyDeServicio, resolveIntegrationsUrl, serverEnv } from '@sytue/auth/server';
import { bff } from '../../../lib/auth';

export const prerender = false;
export const ALL = crearProxyDeServicio({
  bff,
  destino: resolveIntegrationsUrl(serverEnv(import.meta.env)),
  prefijo: '/api/v1/personal-storage/',
  denegar: (path) =>
    !/^\/api\/v1\/personal-storage\/(?:accounts|connect|[0-9a-f-]{36}(?:\/(?:items|content))?)$/.test(
      path,
    ),
});
