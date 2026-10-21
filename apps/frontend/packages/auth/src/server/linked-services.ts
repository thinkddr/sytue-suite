/* SPDX-License-Identifier: Apache-2.0 */
import { bearer, finishAppConsent, HttpClient } from '@sytue/api-clients';
import type { Bff } from './bff.js';
import { rutaDeRetornoDeApp } from './consentimiento.js';
import { crearProxyDeServicio } from './proxy.js';

type Service = 'calendar' | 'contacts' | 'notes';
interface Options {
  bff: Bff;
  destino: string;
  service: Service;
}

/** Each app uses its own host-only session cookie; only these personal-account
 * operations are exposed, never the organisation's admin integration API. */
export function rutasDeServiciosVinculados({ bff, destino, service }: Options) {
  const root = '/api/v1/';
  const operations = {
    calendar: 'collections|events|event|save',
    contacts: 'collections|items|item|save',
    notes: 'collections|items|item|write',
  };
  const allowed = new RegExp(
    `^/api/v1/linked-services/[0-9a-f-]{36}/${service}/(?:${operations[service]})$`,
  );
  return {
    mail: crearProxyDeServicio({
      bff,
      destino,
      prefijo: `${root}external-mail/`,
      denegar: (path) => !/^\/api\/v1\/external-mail\/(?:accounts|connect)$/.test(path),
    }),
    data: crearProxyDeServicio({
      bff,
      destino,
      prefijo: `${root}linked-services/`,
      denegar: (path) => !allowed.test(path),
    }),
    finish: rutaDeRetornoDeApp({
      bff,
      section: `external-${service}`,
      finish: (token, query) =>
        finishAppConsent(
          new HttpClient({ baseUrl: `${destino.replace(/\/$/, '')}/api/v1`, auth: bearer(token) }),
          query,
        ),
    }),
  };
}
