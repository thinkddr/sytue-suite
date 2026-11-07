/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpClient } from './http.js';
import {
  ExternalMailClient,
  isICloudMail,
  mailProviderName,
  forgetMailConsent,
  pendingMailConsent,
  servesMail,
} from './externalMail.js';
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
test('Mail opens only accounts connected for mail', () => {
  expect(servesMail({})).toBe(true);
  expect(servesMail({ services: ['mail', 'calendar'] })).toBe(true);
  expect(servesMail({ services: ['calendar', 'contacts'] })).toBe(false);
  expect(servesMail({ services: [] })).toBe(false);
});

test('iCloud is told apart from other IMAP connections by its server', () => {
  const server = (host: string) => ({
    address: 'ana@icloud.com',
    incoming: { host, port: 993, username: 'ana@icloud.com', security: 'tls' as const },
    outgoing: { host: 'smtp.mail.me.com', port: 587, username: '', security: 'starttls' as const },
  });
  expect(isICloudMail({ provider: 'imap', configuration: server('IMAP.mail.me.com') })).toBe(true);
  expect(isICloudMail({ provider: 'imap', configuration: server('imap.example.eu') })).toBe(false);
  expect(isICloudMail({ provider: 'pop3', configuration: server('imap.mail.me.com') })).toBe(false);
  expect(isICloudMail({ provider: 'imap' })).toBe(false);
});

// The list and the sidebar name an iCloud account as such, not as «IMAP».
test('mailProviderName names the iCloud preset and every provider', () => {
  const icloud = {
    incoming: { host: 'imap.mail.me.com', port: 993, username: '', security: 'tls' as const },
    outgoing: { host: 'smtp.mail.me.com', port: 587, username: '', security: 'starttls' as const },
    address: 'ana@icloud.com',
  };
  expect(mailProviderName({ provider: 'imap', configuration: icloud })).toBe('iCloud Mail');
  expect(mailProviderName({ provider: 'imap' })).toBe('IMAP');
  expect(mailProviderName({ provider: 'yahoo' })).toBe('Yahoo Mail');
});

// Drafts are not listed: the return from a cancelled Outlook sign-in came back with Gmail.
test('the provider of a sign-in in flight survives the trip to the provider', async () => {
  const store = new Map<string, string>();
  const previous = globalThis.sessionStorage;
  globalThis.sessionStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => void store.set(key, value),
    removeItem: (key: string) => void store.delete(key),
  } as Storage;
  try {
    const api = new ExternalMailClient(
      new HttpClient({
        baseUrl: 'https://mail.test/api',
        fetchImpl: async () =>
          Response.json({
            instance_id: 'draft-1',
            authorize_url: 'https://login.microsoftonline.com/authorize?state=sealed',
            scopes: [],
          }),
      }),
    );
    expect(pendingMailConsent('draft-1')).toBeUndefined();
    await api.connect('microsoft', 'external-calendar', undefined, ['mail', 'calendar']);
    const pending = {
      instance_id: 'draft-1',
      provider: 'microsoft',
      services: ['mail', 'calendar'],
    };
    expect(pendingMailConsent('draft-1')).toEqual(pending);
    expect(pendingMailConsent(null)).toEqual(pending);
    expect(pendingMailConsent('other')).toBeUndefined();
    forgetMailConsent();
    expect(pendingMailConsent(null)).toBeUndefined();
    store.set('sytue.external-mail.consent', '{"instance_id":"x","provider":"imap"}');
    expect(pendingMailConsent('x')).toBeUndefined();
    store.set('sytue.external-mail.consent', 'not json');
    expect(pendingMailConsent()).toBeUndefined();
  } finally {
    globalThis.sessionStorage = previous;
  }
});
test('the sidebar summary is one personal GET with no instance in it', async () => {
  const calls: string[] = [];
  const api = new ExternalMailClient(
    new HttpClient({
      baseUrl: 'https://mail.test/api',
      fetchImpl: async (url, init) => {
        calls.push(`${init?.method ?? 'GET'} ${String(url)}`);
        return Response.json({
          items: [
            {
              instance_id: 'own',
              provider: 'imap',
              name: 'Work',
              address: 'ana@work.eu',
              connected: true,
              capabilities: {},
              state: 'reconnect',
              folders: [],
            },
          ],
        });
      },
    }),
  );
  const summary = await api.summary();
  expect(calls).toEqual(['GET https://mail.test/api/external-mail/summary']);
  expect(summary.items[0]?.state).toBe('reconnect');
});
