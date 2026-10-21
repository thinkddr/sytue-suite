/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpClient } from './http.js';
import {
  finishAppConsent,
  disconnectRoomsConnection,
  isRoomsProvider,
  listRoomsConnections,
  ROOMS_PROVIDERS,
  startAppConsent,
} from './appConsent.js';

test('disconnect clears a Rooms grant through deactivate and refuses other products', async () => {
  const changes: string[] = [];
  const client = new HttpClient({
    baseUrl: 'https://gateway.test/api/v1',
    fetchImpl: (async (input, init) => {
      const path = new URL(String(input)).pathname;
      if (init?.method === 'POST') {
        changes.push(path);
        return Response.json({ status: 'disabled' });
      }
      return Response.json(
        path.endsWith('/templates')
          ? { templates: [] }
          : {
              instances: [
                {
                  id: 'rooms',
                  template_key: 'google-chat',
                  name: 'Chat',
                  status: 'active',
                  has_secrets: true,
                  oauth_available: true,
                },
                {
                  id: 'storage',
                  template_key: 'google-drive',
                  name: 'Drive',
                  status: 'active',
                  has_secrets: true,
                },
              ],
            },
      );
    }) as typeof fetch,
  });
  await expect(disconnectRoomsConnection(client, 'storage')).rejects.toThrow(
    'rooms_instance_mismatch',
  );
  await expect(disconnectRoomsConnection(client, 'foreign')).rejects.toThrow(
    'rooms_instance_mismatch',
  );
  expect(changes).toEqual([]);
  await disconnectRoomsConnection(client, 'rooms');
  expect(changes).toEqual(['/api/v1/instances/rooms/deactivate']);
});

test('Rooms starts each provider with a sealed return to Rooms, using the dedicated consent templates', async () => {
  for (const key of Object.keys(ROOMS_PROVIDERS) as (keyof typeof ROOMS_PROVIDERS)[]) {
    const calls: { path: string; body: unknown }[] = [];
    const client = new HttpClient({
      baseUrl: 'https://gateway.test/api/v1',
      fetchImpl: (async (input, init) => {
        const path = new URL(String(input)).pathname;
        calls.push({ path, body: init?.body ? JSON.parse(String(init.body)) : null });
        if (path.endsWith('/oauth/start'))
          return Response.json({ authorize_url: `https://${ROOMS_PROVIDERS[key].host}/authorize` });
        return Response.json(init?.method === 'POST' ? { id: 'draft' } : { instances: [] });
      }) as typeof fetch,
    });
    const result = await startAppConsent(client, ROOMS_PROVIDERS[key], 'rooms');
    expect(calls[1]?.body).toEqual({ template_key: key, name: ROOMS_PROVIDERS[key].name });
    expect(calls[2]?.body).toEqual({ return_to: 'rooms' });
    expect(result.instance_id).toBe('draft');
  }
  for (const key of ['slack', 'microsoft-teams', 'google-meet', '__proto__', null])
    expect(isRoomsProvider(key)).toBe(false);
});

test('Rooms inventory excludes SCIM/SAML accounts and never exposes config or encrypted secrets', async () => {
  const client = new HttpClient({
    baseUrl: 'https://gateway.test/api/v1',
    fetchImpl: (async (input) =>
      Response.json(
        String(input).endsWith('/templates')
          ? {
              templates: [
                { key: 'slack-rooms', manifest: { oauth: { scopes: ['incoming-webhook'] } } },
                { key: 'slack', manifest: { oauth: { scopes: ['admin'] } } },
              ],
            }
          : {
              instances: [
                {
                  id: 'saml',
                  template_key: 'slack',
                  name: 'SCIM account',
                  status: 'active',
                  has_secrets: true,
                },
                {
                  id: 'disabled',
                  template_key: 'slack-rooms',
                  name: 'Old Slack',
                  status: 'disabled',
                  has_secrets: true,
                },
                {
                  id: 'live',
                  template_key: 'slack-rooms',
                  name: 'Slack',
                  status: 'active',
                  has_secrets: true,
                  oauth_available: true,
                  config: { webhook_url: 'secret' },
                },
              ],
            },
      )) as typeof fetch,
  });
  const result = await listRoomsConnections(client);
  expect(result.providers).toEqual([
    {
      key: 'slack-rooms',
      name: 'Slack',
      scopes: ['incoming-webhook'],
      connected: true,
      available: true,
      instance_id: 'live',
    },
  ]);
  expect(result.items.map((item) => item.id)).toEqual(['disabled', 'live']);
  expect(result.items[0]?.connected).toBe(false);
  expect(JSON.stringify(result)).not.toContain('secret');
});

test('Rooms only offers consent for real platform or instance OAuth readiness, including missing metadata', async () => {
  for (const [platform, own, available] of [
    [false, false, false],
    [true, false, true],
    [false, true, true],
    [undefined, undefined, false],
  ] as const) {
    const client = new HttpClient({
      baseUrl: 'https://gateway.test/api/v1',
      fetchImpl: (async (input) =>
        Response.json(
          String(input).endsWith('/templates')
            ? {
                templates: [
                  {
                    key: 'google-chat',
                    oauth_available: platform,
                    manifest: { oauth: { scopes: ['meet'] } },
                  },
                ],
              }
            : {
                instances: [
                  {
                    id: 'own',
                    template_key: 'google-chat',
                    name: 'Meet',
                    status: 'active',
                    has_secrets: true,
                    oauth_available: own,
                  },
                ],
              },
        )) as typeof fetch,
    });
    const inventory = await listRoomsConnections(client);
    expect(inventory.available).toBe(true);
    expect(inventory.providers[0]?.available).toBe(available);
    expect(inventory.providers[0]?.connected).toBe(true);
    expect(inventory.providers[0]?.instance_id).toBe('own');
  }
});

test('Rooms chooses a ready BYO grant ahead of an older unconfigured draft', async () => {
  const paths: string[] = [];
  const client = new HttpClient({
    baseUrl: 'https://gateway.test/api/v1',
    fetchImpl: (async (input) => {
      const path = new URL(String(input)).pathname;
      paths.push(path);
      if (path.endsWith('/templates'))
        return Response.json({
          templates: [
            {
              key: 'google-chat',
              oauth_available: false,
              manifest: { oauth: { scopes: ['meet'] } },
            },
          ],
        });
      if (path.endsWith('/instances'))
        return Response.json({
          instances: [
            {
              id: 'old',
              template_key: 'google-chat',
              name: 'Old',
              status: 'draft',
              has_secrets: false,
              oauth_available: false,
            },
            {
              id: 'ready',
              template_key: 'google-chat',
              name: 'Ready',
              status: 'draft',
              has_secrets: false,
              oauth_available: true,
            },
          ],
        });
      return Response.json({ authorize_url: 'https://accounts.google.com/authorize' });
    }) as typeof fetch,
  });
  const inventory = await listRoomsConnections(client);
  expect(inventory.providers[0]?.instance_id).toBe('ready');
  expect(inventory.providers[0]?.available).toBe(true);
  expect((await startAppConsent(client, ROOMS_PROVIDERS['google-chat'], 'rooms')).instance_id).toBe(
    'ready',
  );
  expect(paths.at(-1)).toBe('/api/v1/instances/ready/oauth/start');
});

test('OAuth completion forwards only the provider callback fields to the gateway', async () => {
  const client = new HttpClient({
    baseUrl: 'https://gateway.test/api/v1',
    fetchImpl: (async (input, init) => {
      expect(String(input)).toBe('https://gateway.test/api/v1/oauth/finish');
      expect(JSON.parse(String(init?.body))).toEqual({
        state: 'sealed',
        code: '',
        error: 'access_denied',
      });
      return Response.json({ section: 'rooms', error: 'access_denied', instance: 'draft' });
    }) as typeof fetch,
  });
  const result = await finishAppConsent(
    client,
    new URLSearchParams({
      state: 'sealed',
      error: 'access_denied',
      return_to: 'https://evil.test',
    }),
  );
  expect(result.section).toBe('rooms');
  expect(result.error).toBe('access_denied');
});

test('Rooms Add account creates a new platform-managed consent for every provider', async () => {
  for (const provider of Object.values(ROOMS_PROVIDERS)) {
    const calls: { path: string; body: unknown }[] = [];
    const client = new HttpClient({
      baseUrl: 'https://gateway.test/api/v1',
      fetchImpl: async (input, init) => {
        const path = new URL(String(input)).pathname;
        calls.push({ path, body: init?.body ? JSON.parse(String(init.body)) : null });
        if (path.endsWith('/templates'))
          return Response.json({ templates: [{ key: provider.template, oauth_available: true }] });
        if (path.endsWith('/oauth/start'))
          return Response.json({ authorize_url: `https://${provider.host}/authorize` });
        if (init?.method === 'POST') return Response.json({ id: 'new-account' });
        throw new Error('Adding an account must not read or reuse the existing identity');
      },
    });
    expect(
      (await startAppConsent(client, provider, 'rooms', undefined, { newAccount: true }))
        .instance_id,
    ).toBe('new-account');
    expect(calls.map((call) => call.path)).toEqual([
      '/api/v1/templates',
      '/api/v1/instances',
      '/api/v1/instances/new-account/oauth/start',
    ]);
    expect(calls.at(-1)?.body).toEqual({ return_to: 'rooms' });
  }
});
