/**
 * /api/snooze, /api/mute y el estado del hilo en /api/thread — los BFF de posponer
 * y silenciar (D-MAIL-07). Traducen el POST del navegador al verbo del servicio
 * (PUT/DELETE), pasan a camelCase y conservan el estado y el `code` de los rechazos
 * que la pantalla sabe contar («solo se puede posponer lo de Recibidos»), en vez de
 * convertirlo todo en un 502. Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () =>
    Promise.resolve({ accessToken: 't', user: { id: 'ada', name: 'Ada', email: 'ada@sytue.eu' } }),
}));

const snooze = await import('../pages/api/snooze.js');
const mute = await import('../pages/api/mute.js');
const thread = await import('../pages/api/thread.js');
const { motivoDePosponer, MailHttpError } = await import('../client/mailApi.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

const llamadas: { metodo: string; ruta: string; cuerpo: unknown }[] = [];

function servicio(respuesta: () => Response): void {
  llamadas.length = 0;
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    llamadas.push({
      metodo: init?.method ?? 'GET',
      ruta: new URL(String(url)).pathname,
      cuerpo: init?.body ? JSON.parse(String(init.body)) : null,
    });
    return respuesta();
  }) as unknown as typeof fetch;
}

const contexto = (request: Request): APIContext =>
  ({ request, url: new URL(request.url) }) as unknown as APIContext;

const post = (ruta: string, cuerpo: unknown) =>
  new Request(`https://mail.sytue.eu${ruta}`, { method: 'POST', body: JSON.stringify(cuerpo) });

const json = (cuerpo: unknown, status = 200) =>
  new Response(JSON.stringify(cuerpo), {
    status,
    headers: { 'content-type': 'application/json' },
  });

const HILO = '0b6f6c2e-6a39-4bd2-9a57-4f2d3c9f5a10';

test('posponer es un PUT al hilo con la fecha en UTC, y vuelve en camelCase', async () => {
  servicio(() => json({ thread_id: HILO, until: '2026-12-01T08:00:00Z' }));
  const res = await snooze.POST(
    contexto(post('/api/snooze', { threadId: HILO, until: '2026-12-01T09:00:00+01:00' })),
  );
  expect(res.status).toBe(200);
  expect(llamadas[0]).toEqual({
    metodo: 'PUT',
    ruta: `/api/v1/threads/${HILO}/snooze`,
    cuerpo: { until: '2026-12-01T08:00:00.000Z' },
  });
  expect(await res.json()).toEqual({ snooze: { threadId: HILO, until: '2026-12-01T08:00:00Z' } });
});

test('quitar la posposición es un DELETE', async () => {
  servicio(() => new Response(null, { status: 204 }));
  const res = await snooze.POST(contexto(post('/api/snooze', { threadId: HILO, cancel: true })));
  expect(res.status).toBe(200);
  expect(llamadas[0]).toMatchObject({ metodo: 'DELETE', ruta: `/api/v1/threads/${HILO}/snooze` });
});

test('el rechazo explicable conserva su estado y su code', async () => {
  servicio(() => json({ error: { code: 'nothing_to_snooze', message: 'x' } }, 409));
  const res = await snooze.POST(
    contexto(post('/api/snooze', { threadId: HILO, until: '2026-12-01T08:00:00Z' })),
  );
  expect(res.status).toBe(409);
  expect(await res.json()).toMatchObject({ code: 'nothing_to_snooze' });
});

test('sin hilo válido ni fecha no se llama al servicio', async () => {
  servicio(() => json({}));
  expect(
    (await snooze.POST(contexto(post('/api/snooze', { threadId: 'x', until: 'y' })))).status,
  ).toBe(400);
  expect((await snooze.POST(contexto(post('/api/snooze', { threadId: HILO })))).status).toBe(400);
  expect((await mute.POST(contexto(post('/api/mute', { threadId: HILO })))).status).toBe(400);
  expect(llamadas).toHaveLength(0);
});

test('la lista de pospuestos va en camelCase', async () => {
  servicio(() => json({ snoozes: [{ thread_id: HILO, until: '2026-12-01T08:00:00Z' }] }));
  const res = await snooze.GET(contexto(new Request('https://mail.sytue.eu/api/snooze')));
  expect(await res.json()).toEqual({
    snoozes: [{ threadId: HILO, until: '2026-12-01T08:00:00Z' }],
  });
});

test('silenciar es un PUT con muted, y devuelve cuántos archivó', async () => {
  servicio(() => json({ muted: true, archived: 2 }));
  const res = await mute.POST(contexto(post('/api/mute', { threadId: HILO, muted: true })));
  expect(llamadas[0]).toEqual({
    metodo: 'PUT',
    ruta: `/api/v1/threads/${HILO}/mute`,
    cuerpo: { muted: true },
  });
  expect(await res.json()).toEqual({ muted: true, archived: 2 });
});

test('el hilo trae su estado: silenciado y hasta cuándo está pospuesto', async () => {
  servicio(() => json({ messages: [], muted: true, snoozed_until: '2026-12-01T08:00:00Z' }));
  const res = await thread.GET(
    contexto(new Request(`https://mail.sytue.eu/api/thread?threadId=${HILO}`)),
  );
  expect(await res.json()).toMatchObject({ muted: true, snoozedUntil: '2026-12-01T08:00:00Z' });
});

test('motivoDePosponer cuenta cada rechazo', () => {
  expect(motivoDePosponer(new MailHttpError(409, 'nothing_to_snooze'))).toBe('nothing');
  expect(motivoDePosponer(new MailHttpError(400, 'invalid_snooze'))).toBe('invalid');
  expect(motivoDePosponer(new MailHttpError(404))).toBe('gone');
  expect(motivoDePosponer(new MailHttpError(502))).toBe('other');
  expect(motivoDePosponer(new Error('red'))).toBe('other');
});
