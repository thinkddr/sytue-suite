/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { Bff } from './bff.js';
import { rutasDeServiciosVinculados } from './linked-services.js';

test('linked app proxy allows only its own personal-data API and requires its session', async () => {
  for (const service of ['calendar', 'contacts', 'notes'] as const) {
    let sessions = 0;
    const bff = {
      basePath: `/${service}`,
      getSession: async () => {
        sessions++;
        return null;
      },
    } as unknown as Bff;
    const routes = rutasDeServiciosVinculados({
      bff,
      destino: 'https://gateway.sytue.invalid',
      service,
    });
    const context = (ruta: string) =>
      ({
        params: { ruta },
        request: new Request(`https://${service}.sytue.invalid/api/${ruta}`),
        url: new URL(`https://${service}.sytue.invalid/api/${ruta}`),
        cookies: { get: () => undefined, set: () => {}, delete: () => {} },
      }) as unknown as APIContext;
    for (const ruta of ['instances', 'oauth/finish', 'accounts/secrets', 'connect/extra'])
      expect((await routes.mail(context(ruta))).status).toBe(403);
    expect(sessions).toBe(0);
    for (const ruta of ['accounts', 'connect'])
      expect((await routes.mail(context(ruta))).status).toBe(401);
    const id = '11111111-1111-4111-8111-111111111111';
    for (const other of ['calendar', 'contacts', 'notes'].filter((value) => value !== service))
      expect((await routes.data(context(`${id}/${other}/collections`))).status).toBe(403);
    expect((await routes.data(context(`${id}/${service}/collections`))).status).toBe(401);
    expect((await routes.data(context(`${id}/${service}/../notes/items`))).status).toBe(400);
    expect((await routes.data(context(`${id}/${service}/collections/secrets`))).status).toBe(403);
    expect(sessions).toBe(3);
  }
});
