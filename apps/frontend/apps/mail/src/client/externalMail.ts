/* SPDX-License-Identifier: Apache-2.0 */
import { ExternalMailClient, HttpClient } from '@sytue/api-clients';
import { withBase } from '@sytue/ui';

export function externalMailClient(): ExternalMailClient {
  return new ExternalMailClient(
    new HttpClient({
      baseUrl: new URL(withBase(import.meta.env.BASE_URL, '/api'), window.location.origin).href,
      fetchImpl: ((url, options) =>
        fetch(url, { ...options, credentials: 'include' })) as typeof fetch,
    }),
  );
}
