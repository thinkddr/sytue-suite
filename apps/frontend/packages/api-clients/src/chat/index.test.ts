// Qué pide el SDK de Chat y qué entiende de lo que le contestan.
//
// No es un cliente huérfano: Sytue Notepads lo usa para «compartir en Rooms»
// (`apps/notes/src/pages/api/suite.ts` y `share.ts`). Con `GET /channels` —una
// ruta que no existe— el selector de canales salía vacío y la acción entera
// era inalcanzable, sin un error a la vista. Medido contra
// `services/rooms/internal/http`. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { ChatClient } from './index.js';

test('WhatsApp consent activates a selected real number through Rooms without provider tokens', async () => {
  const calls: unknown[] = [];
  const client = new ChatClient({
    baseUrl: 'https://rooms.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl: (async (input, init) => {
      calls.push({ path: new URL(String(input)).pathname, body: JSON.parse(String(init?.body)) });
      return Response.json({ account: { id: 'connected' } }, { status: 201 });
    }) as typeof fetch,
  });
  expect(
    await client.connectWhatsAppConsent({
      team_id: 'team',
      instance_id: 'instance',
      phone_number_id: '123',
      pin: '123456',
    }),
  ).toEqual({ account: { id: 'connected' } });
  expect(calls).toEqual([
    {
      path: '/api/v1/whatsapp/consent',
      body: { team_id: 'team', instance_id: 'instance', phone_number_id: '123', pin: '123456' },
    },
  ]);
});

test('el catálogo GIF conserva el cursor y usa Rooms para las imágenes', async () => {
  const calls: string[] = [];
  const controller = new AbortController();
  const client = new ChatClient({
    baseUrl: 'https://rooms.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl: (async (url: string | URL, init?: RequestInit) => {
      const target = new URL(String(url));
      calls.push(`${init?.method ?? 'GET'} ${target.pathname}${target.search}`);
      expect(init?.signal).toBe(controller.signal);
      expect(init?.headers instanceof Headers ? init.headers.get('authorization') : undefined).toBe(
        'Bearer tok',
      );
      if (init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toEqual({ query: 'dance', cursor: '24' });
        expect(init.signal).toBe(controller.signal);
        return Response.json({ items: [], next: '48' });
      }
      return new Response('GIF89a', { headers: { 'content-type': 'image/gif' } });
    }) as typeof fetch,
  });
  expect((await client.searchFreeGifs('dance', '24', controller.signal)).next).toBe('48');
  expect(await (await client.freeGifContent(7, true, controller.signal)).text()).toBe('GIF89a');
  expect(calls).toEqual(['POST /api/v1/gifs/search', 'GET /api/v1/gifs/7?preview=1']);
});

interface Llamada {
  method: string;
  path: string;
  body: unknown;
}

function clienteQueApunta(respuestas: Record<string, unknown>): {
  cliente: ChatClient;
  llamadas: Llamada[];
} {
  const llamadas: Llamada[] = [];
  const fetchImpl = (async (url: string | URL, init?: RequestInit) => {
    const u = new URL(String(url));
    const path = u.pathname.replace('/api/v1', '');
    llamadas.push({
      method: init?.method ?? 'GET',
      path,
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    });
    const cuerpo = respuestas[`${init?.method ?? 'GET'} ${path}`] ?? respuestas[path];
    if (cuerpo === undefined) {
      return new Response(JSON.stringify({ error: 'not found' }), { status: 404 });
    }
    return new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  return {
    cliente: new ChatClient({
      baseUrl: 'https://rooms.test',
      auth: { getAccessToken: () => 'tok' },
      fetchImpl,
    }),
    llamadas,
  };
}

test('listChannels pasa por los equipos, que es donde cuelgan los canales', async () => {
  const { cliente, llamadas } = clienteQueApunta({
    '/teams': { teams: [{ id: 't1' }, { id: 't2' }] },
    '/teams/t1/channels': { channels: [{ id: 'c1', name: 'general', kind: 'public', unread: 3 }] },
    '/teams/t2/channels': { channels: [{ id: 'c2', name: 'diseño', kind: 'private' }] },
  });
  const canales = await cliente.listChannels();
  expect(llamadas.some((l) => l.path === '/channels')).toBe(false);
  expect(canales.map((c) => c.id).sort()).toEqual(['c1', 'c2']);
  // `kind` del servicio → `type` del SDK, y sin no leídos el total es 0, no undefined.
  expect(canales.find((c) => c.id === 'c1')!.type).toBe('public');
  expect(canales.find((c) => c.id === 'c2')!.unread).toBe(0);
});

test('un equipo que falla no se lleva por delante a los demás', async () => {
  const { cliente } = clienteQueApunta({
    '/teams': { teams: [{ id: 't1' }, { id: 'roto' }] },
    '/teams/t1/channels': { channels: [{ id: 'c1', name: 'general', kind: 'public' }] },
  });
  expect((await cliente.listChannels()).map((c) => c.id)).toEqual(['c1']);
});

test('sin equipos no hay canales, y tampoco una excepción', async () => {
  const { cliente } = clienteQueApunta({ '/teams': {} });
  expect(await cliente.listChannels()).toEqual([]);
});

test('unreadTotal suma los no leídos de todos los equipos', async () => {
  const { cliente } = clienteQueApunta({
    '/teams': { teams: [{ id: 't1' }] },
    '/teams/t1/channels': {
      channels: [
        { id: 'c1', name: 'a', kind: 'public', unread: 2 },
        { id: 'c2', name: 'b', kind: 'public', unread: 5 },
      ],
    },
  });
  expect(await cliente.unreadTotal()).toBe(7);
});

test('createChannel crea en el equipo y traduce type → kind', async () => {
  const { cliente, llamadas } = clienteQueApunta({
    '/teams': { teams: [{ id: 't1' }] },
    'POST /teams/t1/channels': { id: 'c9', name: 'nuevo', kind: 'private' },
  });
  const c = await cliente.createChannel({ type: 'private', name: 'nuevo' });
  expect(llamadas[1]!.path).toBe('/teams/t1/channels');
  expect((llamadas[1]!.body as Record<string, unknown>)['kind']).toBe('private');
  expect(c.type).toBe('private');
});

test('listMessages desenvuelve {messages:[…]} y traduce los campos', async () => {
  const { cliente } = clienteQueApunta({
    '/channels/c1/messages': {
      messages: [
        {
          id: 'm1',
          channel_id: 'c1',
          author_sub: 'u1',
          body: 'hola',
          created_at: '2026-09-09T10:00:00Z',
        },
      ],
    },
  });
  const [m] = await cliente.listMessages('c1');
  expect(m!.channelId).toBe('c1');
  expect(m!.authorId).toBe('u1');
  expect(m!.createdAt).toBe('2026-09-09T10:00:00Z');
});

test('markRead manda message_id, que es como lo lee el servicio', async () => {
  const { cliente, llamadas } = clienteQueApunta({ 'POST /channels/c1/read': {} });
  await cliente.markRead('c1', 'm7');
  expect(llamadas[0]!.body).toEqual({ message_id: 'm7' });
});

test('OAuth bridge and Meet methods use the Rooms contract and retain opaque destination IDs', async () => {
  const calls: { method: string; path: string; query: string; body: unknown }[] = [];
  const client = new ChatClient({
    baseUrl: 'https://rooms.test/',
    auth: { getAccessToken: () => 'user-token' },
    fetchImpl: (async (input, init) => {
      const url = new URL(String(input));
      const headers = init?.headers as Headers;
      expect(headers.get('authorization')).toBe('Bearer user-token');
      calls.push({
        method: init?.method ?? 'GET',
        path: url.pathname,
        query: url.search,
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
      });
      if (url.pathname.endsWith('/destinations'))
        return Response.json({ items: [{ id: 'spaces/AAAA', name: 'Project' }] });
      if (url.pathname.endsWith('/bridges'))
        return Response.json({ bridge: { id: 'bridge-1' } }, { status: 201 });
      return Response.json({}, { status: 404 });
    }) as typeof fetch,
  });
  expect(await client.integrationDestinations('account id')).toEqual({
    items: [{ id: 'spaces/AAAA', name: 'Project' }],
  });
  expect(
    await client.connectOAuthBridge('channel/one', 'account-1', 'team:one/channel:two', 'teams'),
  ).toEqual({ bridge: { id: 'bridge-1' } });
  expect(calls).toEqual([
    {
      method: 'GET',
      path: '/api/v1/integrations/destinations',
      query: '?instance=account+id',
      body: undefined,
    },
    {
      method: 'POST',
      path: '/api/v1/channels/channel%2Fone/bridges',
      query: '',
      body: { instance_id: 'account-1', remote_id: 'team:one/channel:two', platform: 'teams' },
    },
  ]);
});

test('OAuth destinations do not turn a backend permission failure into an empty list', async () => {
  const client = new ChatClient({
    baseUrl: 'https://rooms.test',
    auth: { getAccessToken: () => 'user-token' },
    fetchImpl: (async () =>
      Response.json(
        { error: { code: 'forbidden', message: 'Only admins may connect accounts.' } },
        { status: 403 },
      )) as typeof fetch,
  });
  await expect(client.integrationDestinations('account-1')).rejects.toMatchObject({ status: 403 });
});

test('Matrix native broker keeps tokens in authenticated JSON requests and preserves PKCE', async () => {
  const calls: { path: string; method: string; body: unknown; query: string }[] = [];
  const client = new ChatClient({
    baseUrl: 'https://rooms.test',
    auth: { getAccessToken: () => 'sytue-native-session' },
    fetchImpl: (async (input, init) => {
      const url = new URL(String(input));
      expect(new Headers(init?.headers).get('authorization')).toBe('Bearer sytue-native-session');
      calls.push({
        path: url.pathname,
        method: init?.method ?? 'GET',
        body: init?.body ? JSON.parse(String(init.body)) : null,
        query: url.search,
      });
      return Response.json({});
    }) as typeof fetch,
  });
  await client.startMatrixNative({ code_challenge: 'challenge', code_challenge_method: 'S256' });
  await client.previewMatrixNative('opaque-state');
  await client.acceptMatrixNative({ state: 'opaque-state', login_token: 'one-use-Matrix-token' });
  await client.finishMatrixNative({
    state: 'opaque-state',
    ticket: 'opaque-ticket',
    code_verifier: 'native-verifier',
  });
  expect(calls).toEqual([
    {
      path: '/api/v1/matrix/native/start',
      method: 'POST',
      body: { code_challenge: 'challenge', code_challenge_method: 'S256' },
      query: '',
    },
    {
      path: '/api/v1/matrix/native/preview',
      method: 'GET',
      body: null,
      query: '?state=opaque-state',
    },
    {
      path: '/api/v1/matrix/native/accept',
      method: 'POST',
      body: { state: 'opaque-state', login_token: 'one-use-Matrix-token' },
      query: '',
    },
    {
      path: '/api/v1/matrix/native/finish',
      method: 'POST',
      body: { state: 'opaque-state', ticket: 'opaque-ticket', code_verifier: 'native-verifier' },
      query: '',
    },
  ]);
});
