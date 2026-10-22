/* SPDX-License-Identifier: Apache-2.0 */
import { resolveIntegrationsUrl, serverEnv } from '@sytue/auth/server';
import { bearer, HttpClient } from '@sytue/api-clients';

export function appConnectionsClient(accessToken: string): HttpClient {
  return new HttpClient({
    baseUrl: `${resolveIntegrationsUrl(serverEnv(import.meta.env)).replace(/\/$/, '')}/api/v1`,
    auth: bearer(accessToken),
  });
}
