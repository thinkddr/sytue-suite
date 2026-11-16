/**
 * «¿Vas a ir?» desde la vista rápida: el BFF manda el UID del evento al servicio.
 *
 * La web solo conoce el UID del .ics («…@calendar.sytue.eu») y el servicio pedía
 * un UUID en la ruta: toda respuesta acababa en 400 y en «No se ha podido enviar
 * la respuesta». Ahora el UID viaja en el cuerpo, que es lo que el servicio lee
 * antes que la ruta (un UID con forma de UUID se tomaría allí por el id interno).
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, describe, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () => Promise.resolve({ accessToken: 't', user: { id: 'ana' } }),
}));

const { POST } = await import('../pages/api/partstat.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

function responder(body: unknown): Promise<Response> {
  const request = new Request('https://calendar.sytue.eu/calendar/api/partstat', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
  return POST({ request } as unknown as APIContext) as Promise<Response>;
}

describe('/api/partstat', () => {
  test('manda el UID en el cuerpo y en la ruta, con la respuesta en mayúsculas', async () => {
    const pedidas: { url: string; body: unknown; method: string }[] = [];
    globalThis.fetch = ((url: string, init: RequestInit) => {
      pedidas.push({ url, method: init.method ?? '', body: JSON.parse(String(init.body)) });
      return Promise.resolve(new Response('{}', { status: 200 }));
    }) as unknown as typeof fetch;

    const res = await responder({ id: 'reunion-1@calendar.sytue.eu', status: 'tentative' });
    expect(res.status).toBe(200);
    expect(pedidas).toHaveLength(1);
    expect(pedidas[0]?.method).toBe('PUT');
    expect(pedidas[0]?.url).toContain('/events/reunion-1%40calendar.sytue.eu/attendees/partstat');
    expect(pedidas[0]?.body).toEqual({ uid: 'reunion-1@calendar.sytue.eu', partstat: 'TENTATIVE' });
  });

  test('un UID con saltos de línea no llega al servicio', async () => {
    let llamadas = 0;
    globalThis.fetch = (() => {
      llamadas++;
      return Promise.resolve(new Response('{}', { status: 200 }));
    }) as unknown as typeof fetch;
    const res = await responder({ id: 'a\r\nX-MAL:1', status: 'accepted' });
    expect(res.status).toBe(400);
    expect(llamadas).toBe(0);
  });

  test('«no figuras como invitado» (404) se propaga tal cual', async () => {
    globalThis.fetch = (() =>
      Promise.resolve(new Response('{}', { status: 404 }))) as unknown as typeof fetch;
    expect((await responder({ id: 'ajeno@calendar.sytue.eu', status: 'declined' })).status).toBe(
      404,
    );
  });
});
