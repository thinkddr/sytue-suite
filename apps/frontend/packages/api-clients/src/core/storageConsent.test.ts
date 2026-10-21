/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpClient } from './http.js';
import { isStorageOAuthKind, startStorageConsent, STORAGE_PROVIDERS } from './storageConsent.js';

test('only cloud providers can start OAuth; object prototype keys are rejected', () => {
  for (const value of ['s3', 'webdav', 'nas', '__proto__', 'constructor', null]) {
    expect(isStorageOAuthKind(value)).toBe(false);
  }
});

test('each cloud provider creates its own instance and returns through the storage wizard', async () => {
  for (const kind of ['gdrive', 'onedrive', 'dropbox'] as const) {
    const calls: { path: string; body: unknown }[] = [];
    const client = new HttpClient({
      baseUrl: 'https://gateway.test/api/v1',
      fetchImpl: (async (input, init) => {
        const path = new URL(String(input)).pathname;
        calls.push({ path, body: init?.body ? JSON.parse(String(init.body)) : null });
        if (path.endsWith('/oauth/start'))
          return Response.json({
            authorize_url: `https://${STORAGE_PROVIDERS[kind].host}/authorize?state=sealed`,
          });
        return Response.json(init?.method === 'POST' ? { id: 'new-instance' } : { instances: [] });
      }) as typeof fetch,
    });
    const result = await startStorageConsent(client, kind);
    expect(calls.map((call) => call.path)).toEqual([
      '/api/v1/instances',
      '/api/v1/instances',
      '/api/v1/instances/new-instance/oauth/start',
    ]);
    expect(calls[1]?.body).toEqual({
      template_key: STORAGE_PROVIDERS[kind].template,
      name: STORAGE_PROVIDERS[kind].name,
    });
    expect(calls[2]?.body).toEqual({ return_to: 'storage' });
    expect(result.instance_id).toBe('new-instance');
  }
});

test('a matching draft is reused; a foreign provider instance cannot be authorized', async () => {
  const calls: string[] = [];
  const client = new HttpClient({
    baseUrl: 'https://gateway.test/api/v1',
    fetchImpl: (async (input) => {
      const path = new URL(String(input)).pathname;
      calls.push(path);
      return Response.json(
        path.endsWith('/oauth/start')
          ? { authorize_url: 'https://www.dropbox.com/oauth2/authorize' }
          : {
              instances: [{ id: 'draft', template_key: 'dropbox-storage', status: 'draft' }],
            },
      );
    }) as typeof fetch,
  });
  await startStorageConsent(client, 'dropbox', 'draft');
  expect(calls).toEqual(['/api/v1/instances', '/api/v1/instances/draft/oauth/start']);
  await expect(startStorageConsent(client, 'gdrive', 'draft')).rejects.toThrow(
    'storage_instance_mismatch',
  );
});

test('authorization redirects must belong to the selected HTTPS provider', async () => {
  for (const authorize_url of [
    'https://evil.test/auth',
    'http://www.dropbox.com/auth',
    'https://user@www.dropbox.com/auth',
    'https://www.dropbox.com:8443/auth',
    'https://www.dropbox.com/auth#untrusted',
  ]) {
    const client = new HttpClient({
      baseUrl: 'https://gateway.test/api/v1',
      fetchImpl: (async (input) =>
        Response.json(
          String(input).endsWith('/oauth/start')
            ? { authorize_url }
            : { instances: [{ id: 'draft', template_key: 'dropbox-storage', status: 'draft' }] },
        )) as typeof fetch,
    });
    await expect(startStorageConsent(client, 'dropbox')).rejects.toThrow(
      'invalid_storage_authorization_url',
    );
  }
});

test('adding a cloud account creates a fresh instance without reusing or reading the old identity', async () => {
  for (const kind of ['gdrive', 'onedrive', 'dropbox'] as const) {
    const calls: { path: string; method?: string; body: unknown }[] = [];
    const client = new HttpClient({
      baseUrl: 'https://gateway.test/api/v1',
      fetchImpl: (async (input, init) => {
        const path = new URL(String(input)).pathname;
        calls.push({
          path,
          method: init?.method,
          body: init?.body ? JSON.parse(String(init.body)) : null,
        });
        if (path.endsWith('/templates'))
          return Response.json({
            templates: [{ key: STORAGE_PROVIDERS[kind].template, oauth_available: true }],
          });
        if (path.endsWith('/oauth/start'))
          return Response.json({
            authorize_url: `https://${STORAGE_PROVIDERS[kind].host}/authorize?state=new-account`,
          });
        if (init?.method === 'POST') return Response.json({ id: 'fresh' });
        throw new Error('New consent must not read or reuse an existing account');
      }) as typeof fetch,
    });
    const result = await startStorageConsent(client, kind, undefined, { newAccount: true });
    expect(result.instance_id).toBe('fresh');
    expect(calls.map(({ path }) => path)).toEqual([
      '/api/v1/templates',
      '/api/v1/instances',
      '/api/v1/instances/fresh/oauth/start',
    ]);
    expect(calls[2]?.body).toEqual({ return_to: 'storage' });
  }
});

test('new cloud accounts require real platform OAuth readiness and do not clone a private client', async () => {
  const calls: string[] = [];
  const client = new HttpClient({
    baseUrl: 'https://gateway.test/api/v1',
    fetchImpl: (async (input) => {
      const path = new URL(String(input)).pathname;
      calls.push(path);
      return Response.json({ templates: [{ key: 'google-drive', oauth_available: false }] });
    }) as typeof fetch,
  });
  await expect(
    startStorageConsent(client, 'gdrive', undefined, { newAccount: true }),
  ).rejects.toThrow('OAuth provider unavailable');
  expect(calls).toEqual(['/api/v1/templates']);
  await expect(
    startStorageConsent(client, 'gdrive', 'existing', { newAccount: true }),
  ).rejects.toThrow('storage_instance_mismatch');
  expect(calls).toEqual(['/api/v1/templates']);
});
