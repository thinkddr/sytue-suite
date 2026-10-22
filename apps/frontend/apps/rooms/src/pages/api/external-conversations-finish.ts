/* SPDX-License-Identifier: Apache-2.0 */
import { rutaDeRetornoDeApp } from '@sytue/auth/server';
import { finishAppConsent } from '@sytue/api-clients';
import { bff } from '../../lib/bff.js';
import { appConnectionsClient } from '../../lib/app-connections.js';

export const prerender = false;
export const GET = rutaDeRetornoDeApp({
  bff,
  section: 'external-chat',
  finish: (accessToken, query) => finishAppConsent(appConnectionsClient(accessToken), query),
});
