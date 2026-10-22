/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpClient, HttpError } from './http.js';
import { LinkedServicesClient } from './linkedServices.js';
import type { ExternalMailAccount } from './externalMail.js';
const account = { instance_id: 'exact', address: 'fixture@example.eu' } as ExternalMailAccount;
test('provider writes bind identity/version, keep partial fields and never replay an unknown outcome', async () => {
  for (const mode of ['network', 'truncated', 'proxy', 'conflict']) {
    let calls = 0;
    const api = new LinkedServicesClient(
      new HttpClient({
        baseUrl: 'https://app.test/api',
        fetchImpl: async (_url, options) => {
          calls++;
          expect(JSON.parse(String(options?.body))).toEqual({
            collection: 'collection',
            id: 'event',
            etag: '"version"',
            patch: { summary: 'Updated' },
            expected_address: account.address,
          });
          if (mode === 'network') throw new TypeError('synthetic connection reset');
          if (mode === 'truncated') return new Response('{', { status: 200 });
          return Response.json(
            { error: { code: mode === 'conflict' ? 'external_resource_changed' : 'unavailable' } },
            { status: mode === 'conflict' ? 409 : 503 },
          );
        },
      }),
    );
    try {
      await api.saveEvent(account, 'collection', 'event', '"version"', { summary: 'Updated' });
      throw new Error('expected failure');
    } catch (error) {
      expect(error).toBeInstanceOf(HttpError);
      expect((error as HttpError).code).toBe(
        mode === 'conflict' ? 'external_resource_changed' : 'external_write_unknown',
      );
    }
    expect(calls).toBe(1);
  }
});
