/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpClient } from './http.js';
import { ExternalMailClient } from './externalMail.js';
test('mail consent uses the personal route and exact selected provider origin', async () => {
  for (const [provider, host] of [
    ['gmail', 'accounts.google.com'],
    ['microsoft', 'login.microsoftonline.com'],
    ['yahoo', 'api.login.yahoo.com'],
  ] as const) {
    let body: unknown;
    const api = new ExternalMailClient(
      new HttpClient({
        baseUrl: 'https://mail.test/api',
        fetchImpl: async (url, init) => {
          expect(String(url)).toBe('https://mail.test/api/external-mail/connect');
          body = JSON.parse(String(init?.body));
          return Response.json({
            instance_id: 'own',
            authorize_url: `https://${host}/authorize?state=sealed`,
            scopes: [],
          });
        },
      }),
    );
    await api.connect(provider, 'mail', 'existing');
    expect(body).toEqual({ provider, return_to: 'mail', instance_id: 'existing' });
  }
});
test('mail consent rejects credential phishing origins', async () => {
  for (const target of [
    'http://accounts.google.com/authorize',
    'https://accounts.google.com.evil.test/',
    'https://user:secret@accounts.google.com/',
    'https://accounts.google.com:8443/',
    'https://accounts.google.com/#token',
    'https://login.microsoftonline.com/',
  ]) {
    const api = new ExternalMailClient(
      new HttpClient({
        baseUrl: 'https://mail.test/api',
        fetchImpl: async () => Response.json({ instance_id: 'own', authorize_url: target }),
      }),
    );
    await expect(api.connect('gmail')).rejects.toThrow('invalid_mail_authorization_url');
  }
});
test('mail actions keep opaque IDs as query data and never replay sending', async () => {
  const calls: { url: URL; body: unknown }[] = [];
  const api = new ExternalMailClient(
    new HttpClient({
      baseUrl: 'https://mail.test/api',
      fetchImpl: async (raw, init) => {
        const url = new URL(String(raw));
        calls.push({ url, body: init?.body ? JSON.parse(String(init.body)) : null });
        if (url.pathname.endsWith('/send'))
          return Response.json({ error: { code: 'mail_send_unknown' } }, { status: 502 });
        return Response.json({ items: [] });
      },
    }),
  );
  await api.messages('account', 'Folder/with+id', 'opaque+cursor', undefined, 'fixture@example.eu');
  expect(calls[0]?.url.searchParams.get('folder')).toBe('Folder/with+id');
  expect(calls[0]?.url.searchParams.get('cursor')).toBe('opaque+cursor');
  expect(calls[0]?.url.searchParams.get('expected_address')).toBe('fixture@example.eu');
  await api.action('account', 'INBOX', 'id', 'trash', undefined, 'fixture@example.eu');
  expect(calls[1]?.body).toMatchObject({ expected_address: 'fixture@example.eu', action: 'trash' });
  await expect(
    api.send('account', {
      expected_address: 'fixture@example.eu',
      to: [{ email: 'to@example.eu' }],
      subject: 'fixture',
      text: 'fixture',
    }),
  ).rejects.toThrow();
  expect(calls.filter((call) => call.url.pathname.endsWith('/send'))).toHaveLength(1);
  expect(calls.every((call) => call.url.pathname.startsWith('/api/external-mail/'))).toBe(true);
});
