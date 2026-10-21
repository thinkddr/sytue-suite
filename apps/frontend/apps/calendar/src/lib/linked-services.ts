/* SPDX-License-Identifier: Apache-2.0 */
import { rutasDeServiciosVinculados, resolveIntegrationsUrl, serverEnv } from '@sytue/auth/server';
import { bff } from './bff.js';

export const linkedServices = rutasDeServiciosVinculados({
  bff,
  destino: resolveIntegrationsUrl(serverEnv(import.meta.env)),
  service: 'calendar',
});
