/* SPDX-License-Identifier: Apache-2.0 */
import { rutaDeRetornoDeApp } from '@sytue/auth/server';
import { HttpClient, bearer, finishAppConsent } from '@sytue/api-clients';
import { bff, integrationsBaseUrl } from '../../lib/auth';

export const prerender = false;
export const GET = rutaDeRetornoDeApp({
  bff,
  section: 'personal-storage',
  finish: (accessToken, query) =>
    finishAppConsent(
      new HttpClient({
        baseUrl: `${integrationsBaseUrl().replace(/\/$/, '')}/api/v1`,
        auth: bearer(accessToken),
      }),
      query,
    ),
});
