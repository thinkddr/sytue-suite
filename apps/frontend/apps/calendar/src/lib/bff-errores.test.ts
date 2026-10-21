/**
 * El BFF propaga el 4xx del servicio TAL CUAL, y la vista no cae por un calendario
 * al que ya no hay acceso.
 *
 * Visto en producción (v3.6.27): retirada la compartición, `REPORT` del calendario de
 * la dueña da 403 y `/api/events` lo convertía en 502. La web lo tomaba por una caída,
 * pintaba el error genérico de toda la vista y lo volvía a pedir en cada cambio de rango.
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, describe, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';
import { HttpError } from '@sytue/api-clients';

mock.module('./session.js', () => ({
  getApiSession: () => Promise.resolve({ accessToken: 't', user: { id: 'lectora' } }),
}));

const { GET } = await import('../pages/api/events.js');
const { listEventsOf } = await import('../client/calendarApi.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

function servicioQueDa(status: number): void {
  globalThis.fetch = (() =>
    Promise.resolve(new Response('{"error":"x"}', { status }))) as unknown as typeof fetch;
}

function pedirEventos(): Promise<Response> {
  const url = new URL(
    'https://calendar.sytue.eu/calendar/api/events?calendarUrl=/dav/calendars/duena/personal/',
  );
  return GET({ url, request: new Request(url) } as unknown as APIContext) as Promise<Response>;
}

describe('/api/events', () => {
  for (const status of [401, 403, 404]) {
    test(`un ${status} del servicio sale como ${status}, no como 502`, async () => {
      servicioQueDa(status);
      const res = await pedirEventos();
      expect(res.status).toBe(status);
      // Sin el texto de la excepción: llevaba la URL INTERNA del Service.
      expect(await res.text()).not.toContain('HTTP ');
    });
  }

  test('un 5xx del servicio sí es un 502', async () => {
    servicioQueDa(500);
    expect((await pedirEventos()).status).toBe(502);
  });
});

describe('listEventsOf', () => {
  const cal = (url: string, displayName: string) => ({ url, displayName });
  const propio = cal('/dav/calendars/lectora/personal/', 'Personal');
  const retirado = cal('/dav/calendars/duena/personal/', 'Equipo');

  test('un 403 de un calendario lo aparta sin tumbar los demás', async () => {
    const r = await listEventsOf([propio, retirado], undefined, (url) =>
      url === retirado.url
        ? Promise.reject(new HttpError(403, 'Forbidden', url))
        : Promise.resolve({ events: [{ href: 'e1' } as never] }),
    );
    expect(r.sinAcceso).toEqual([retirado]);
    expect(r.byCal[propio.url]).toHaveLength(1);
  });

  test('un 404 también: el calendario lo borraron en otro sitio', async () => {
    const r = await listEventsOf([retirado], undefined, (url) =>
      Promise.reject(new HttpError(404, 'Not Found', url)),
    );
    expect(r.sinAcceso).toEqual([retirado]);
  });

  test('lo demás sigue siendo un error de la vista', async () => {
    const promesa = listEventsOf([propio], undefined, (url) =>
      Promise.reject(new HttpError(502, 'Bad Gateway', url)),
    );
    await expect(promesa).rejects.toBeInstanceOf(HttpError);
  });
});
