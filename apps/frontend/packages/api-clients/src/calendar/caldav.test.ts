// Lo que el SDK CalDAV pide al servicio y lo que devuelve de su respuesta.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { CalDavClient } from './caldav.js';

test('getEvent lee el recurso tal cual con su ETag, para fundir encima al editar', async () => {
  const pedidas: { method: string; url: string; auth: string | null }[] = [];
  const cliente = new CalDavClient({
    baseUrl: 'https://calendar.test',
    userId: 'yo',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl: (async (url: string | URL, init?: RequestInit) => {
      pedidas.push({
        method: init?.method ?? 'GET',
        url: String(url),
        auth: new Headers(init?.headers).get('authorization'),
      });
      return new Response('BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n', {
        status: 200,
        headers: { etag: '"e1"', 'content-type': 'text/calendar' },
      });
    }) as unknown as typeof fetch,
  });
  const r = await cliente.getEvent('/dav/calendars/yo/personal/a.ics');
  expect(r).toEqual({ ical: 'BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n', etag: '"e1"' });
  expect(pedidas).toEqual([
    {
      method: 'GET',
      url: 'https://calendar.test/dav/calendars/yo/personal/a.ics',
      auth: 'Bearer tok',
    },
  ]);
});

test('getEvent no manda el Bearer fuera del servicio', async () => {
  const cliente = new CalDavClient({
    baseUrl: 'https://calendar.test',
    userId: 'yo',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl: (() => Promise.resolve(new Response(''))) as unknown as typeof fetch,
  });
  await expect(cliente.getEvent('https://otro.example/x.ics')).rejects.toThrow();
});
