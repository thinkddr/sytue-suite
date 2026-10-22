/* SPDX-License-Identifier: Apache-2.0 */
import { rutaDeRetornoDeApp, resolveIntegrationsUrl, serverEnv } from '@sytue/auth/server';
import { HttpClient, bearer, finishAppConsent } from '@sytue/api-clients';
import { bff } from '../../lib/bff.js';

export const prerender = false;
export const GET = rutaDeRetornoDeApp({
  bff,
  section: 'external-mail',
  finish: (accessToken, query) =>
    finishAppConsent(
      new HttpClient({
        baseUrl: `${resolveIntegrationsUrl(serverEnv(import.meta.env)).replace(/\/$/, '')}/api/v1`,
        auth: bearer(accessToken),
      }),
      query,
    ),
});
