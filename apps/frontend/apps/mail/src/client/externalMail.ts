/* SPDX-License-Identifier: Apache-2.0 */
import { ExternalMailClient, HttpClient } from '@sytue/api-clients';
import { withBase } from '@sytue/ui';

// MailApp, ComposeModal y ExternalMail lo crean en un `useMemo`, que TAMBIÉN corre al
// renderizar en el servidor, donde no hay `window`: leer `window.location` aquí tumbó
// todas las páginas de Mail con sesión («Internal server error», v3.19.3). En el
// servidor nunca se hace una petición (los efectos no corren), así que basta un
// origen que no resuelva: si alguna se colara, fallaría a la vista.
const ORIGEN_SSR = 'http://ssr.invalid';

export function externalMailClient(): ExternalMailClient {
  const origen = typeof window === 'undefined' ? ORIGEN_SSR : window.location.origin;
  return new ExternalMailClient(
    new HttpClient({
      baseUrl: new URL(withBase(import.meta.env.BASE_URL, '/api'), origen).href,
      fetchImpl: ((url, options) =>
        fetch(url, { ...options, credentials: 'include' })) as typeof fetch,
    }),
  );
}
