/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpClient } from './http.js';
import { ExternalConversationsClient } from './externalConversations.js';

test('external consent accepts only the selected provider HTTPS origin', async () => {
  for (const url of [
    'https://slack.com.evil.test/oauth/v2/authorize',
    'http://slack.com/oauth/v2/authorize',
    'https://user:secret@slack.com/oauth/v2/authorize',
    'https://login.microsoftonline.com/authorize',
    'https://slack.com:8443/oauth/v2/authorize',
    'https://slack.com/oauth/v2/authorize#token',
  ]) {
    const client = new ExternalConversationsClient(
      new HttpClient({
        baseUrl: 'https://rooms.test/api',
        fetchImpl: async () =>
          Response.json({ authorize_url: url, instance_id: 'account', scopes: [] }),
      }),
    );
    await expect(client.connect('slack')).rejects.toThrow('invalid_external_authorization_url');
  }
});

test('external threads and actions preserve the provider IDs and never call native message routes', async () => {
  const calls: { url: URL; init: RequestInit | undefined }[] = [];
  const client = new ExternalConversationsClient(
    new HttpClient({
      baseUrl: 'https://rooms.test/api',
      fetchImpl: async (url, init) => {
        calls.push({ url: new URL(String(url)), init });
        return Response.json({ items: [], capabilities: {} });
      },
    }),
  );
  await client.messages('account', 'spaces/space', {
    thread: 'spaces/space/threads/thread',
    cursor: 'cursor+opaque',
  });
  await client.send(
    'account',
    'spaces/space',
    'Private provider content',
    'spaces/space/threads/thread',
  );
  await client.action(
    'account',
    'edit',
    'spaces/space',
    'spaces/space/messages/message',
    'Edited provider content',
  );
  expect(calls.map((c) => c.url.pathname)).toEqual([
    '/api/external-conversations/account/messages',
    '/api/external-conversations/account/messages',
    '/api/external-conversations/account/actions',
  ]);
  expect(calls[0]!.url.searchParams.get('thread_id')).toBe('spaces/space/threads/thread');
  expect(calls[0]!.url.searchParams.get('cursor')).toBe('cursor+opaque');
  expect(JSON.parse(String(calls[1]!.init?.body))).toEqual({
    remote_id: 'spaces/space',
    text: 'Private provider content',
    thread_id: 'spaces/space/threads/thread',
  });
});

// A provider may have accepted a POST before the network fails. Never replay it.
test('an ambiguous provider send is attempted once', async () => {
  let calls = 0;
  const client = new ExternalConversationsClient(
    new HttpClient({
      baseUrl: 'https://rooms.test/api',
      fetchImpl: async () => {
        calls++;
        throw new TypeError('connection lost');
      },
    }),
  );
  await expect(client.send('account', 'C123', 'message')).rejects.toThrow('connection lost');
  expect(calls).toBe(1);
});

test('Meet creates with the explicitly selected account and preserves legacy single-account requests', async () => {
  const bodies: unknown[] = [];
  const client = new ExternalConversationsClient(
    new HttpClient({
      baseUrl: 'https://rooms.test/api',
      fetchImpl: async (_url, init) => {
        bodies.push(JSON.parse(String(init?.body)));
        return Response.json({ url: 'https://meet.google.com/abc-defg-hij' });
      },
    }),
  );
  await client.meeting('selected-second-account');
  await client.meeting();
  expect(bodies).toEqual([
    { provider: 'google-meet', instance_id: 'selected-second-account' },
    { provider: 'google-meet' },
  ]);
});

test('Teams meeting consent extends the selected vault with the meeting purpose', async () => {
  let body: unknown;
  const client = new ExternalConversationsClient(
    new HttpClient({
      baseUrl: 'https://rooms.test/api',
      fetchImpl: async (_url, init) => {
        body = JSON.parse(String(init?.body));
        return Response.json({
          authorize_url:
            'https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=nonce',
          instance_id: 'existing-chat',
          scopes: ['OnlineMeetings.ReadWrite'],
        });
      },
    }),
  );
  await client.connectMeeting('microsoft-teams', 'existing-chat');
  expect(body).toEqual({
    provider: 'microsoft-teams',
    instance_id: 'existing-chat',
    return_to: 'rooms',
    purpose: 'meeting',
  });
});

test('Teams create and update require the verified protected result and preserve exact identity', async () => {
  const calls: { method: string; body: unknown }[] = [];
  const result = {
    provider: 'microsoft-teams',
    meeting_id: 'opaque',
    url: 'https://teams.microsoft.com/l/meetup-join/opaque?context=encoded',
    e2ee: true,
    official_client_required: true,
  };
  let output: unknown = result;
  const client = new ExternalConversationsClient(
    new HttpClient({
      baseUrl: 'https://rooms.test/api',
      fetchImpl: async (_url, init) => {
        calls.push({ method: init!.method!, body: JSON.parse(String(init?.body)) });
        return Response.json(output);
      },
    }),
  );
  expect(await client.protectedTeamsMeeting('selected', 'account')).toEqual(result);
  await client.protectedTeamsMeeting('selected', 'account', 'opaque');
  expect(calls).toEqual([
    {
      method: 'POST',
      body: {
        provider: 'microsoft-teams',
        instance_id: 'selected',
        expected_account_id: 'account',
        e2ee: true,
      },
    },
    {
      method: 'PATCH',
      body: {
        provider: 'microsoft-teams',
        instance_id: 'selected',
        expected_account_id: 'account',
        e2ee: true,
        meeting_id: 'opaque',
      },
    },
  ]);
  for (const invalid of [
    { ...result, e2ee: false },
    { ...result, e2ee: undefined },
    { ...result, official_client_required: false },
    { ...result, url: 'https://teams.microsoft.com.evil.test/meet/opaque' },
    { ...result, meeting_id: 'other' },
  ]) {
    output = invalid;
    await expect(client.protectedTeamsMeeting('selected', 'account', 'opaque')).rejects.toThrow();
  }
});

test('an ambiguous protected meeting create is never replayed', async () => {
  let writes = 0;
  const client = new ExternalConversationsClient(
    new HttpClient({
      baseUrl: 'https://rooms.test/api',
      fetchImpl: async () => {
        writes++;
        throw new TypeError('lost after acceptance');
      },
    }),
  );
  await expect(client.protectedTeamsMeeting('selected', 'account')).rejects.toThrow(
    'lost after acceptance',
  );
  expect(writes).toBe(1);
});
