/**
 * /api/reading-prefs y /api/trusted-senders — los BFF de la lectura (D-MAIL-08).
 * Traducen el POST del navegador al verbo del servicio (PUT / POST / DELETE), pasan
 * las preferencias entre camelCase y snake_case, llevan la dirección a quitar en la
 * query codificada y conservan el 400 del servicio en vez de convertirlo en un 502.
 * Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () =>
    Promise.resolve({ accessToken: 't', user: { id: 'ada', name: 'Ada', email: 'ada@sytue.eu' } }),
}));

const prefs = await import('../pages/api/reading-prefs.js');
const confianza = await import('../pages/api/trusted-senders.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

const llamadas: { metodo: string; ruta: string; query: string; cuerpo: unknown }[] = [];

function servicio(respuesta: () => Response): void {
  llamadas.length = 0;
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    const u = new URL(String(url));
    llamadas.push({
      metodo: init?.method ?? 'GET',
      ruta: u.pathname,
      query: u.search,
      cuerpo: init?.body ? JSON.parse(String(init.body)) : null,
    });
    return respuesta();
  }) as unknown as typeof fetch;
}

const contexto = (request: Request): APIContext =>
  ({ request, url: new URL(request.url) }) as unknown as APIContext;
const post = (ruta: string, cuerpo: unknown) =>
  new Request(`https://mail.sytue.eu${ruta}`, { method: 'POST', body: JSON.stringify(cuerpo) });

test('GET de las preferencias las pasa a camelCase', async () => {
  servicio(() => Response.json({ auto_advance: 'newer', swipe_right: 'read', swipe_left: 'none' }));
  const res = await prefs.GET(contexto(new Request('https://mail.sytue.eu/api/reading-prefs')));
  expect(llamadas[0]).toMatchObject({ metodo: 'GET', ruta: '/api/v1/reading-prefs' });
  expect(await res.json()).toEqual({ autoAdvance: 'newer', swipeRight: 'read', swipeLeft: 'none' });
});

test('POST de las preferencias es un PUT al servicio, y su 400 se conserva', async () => {
  servicio(() => new Response(null, { status: 204 }));
  const ok = await prefs.POST(
    contexto(
      post('/api/reading-prefs', {
        autoAdvance: 'list',
        swipeRight: 'trash',
        swipeLeft: 'archive',
      }),
    ),
  );
  expect(ok.status).toBe(200);
  expect(llamadas[0]).toEqual({
    metodo: 'PUT',
    ruta: '/api/v1/reading-prefs',
    query: '',
    cuerpo: { auto_advance: 'list', swipe_right: 'trash', swipe_left: 'archive' },
  });
  servicio(() => Response.json({ error: { code: 'invalid_reading_prefs' } }, { status: 400 }));
  const mal = await prefs.POST(contexto(post('/api/reading-prefs', { autoAdvance: 'x' })));
  expect(mal.status).toBe(400);
});

test('confiar es un POST, y dejar de confiar un DELETE con la dirección en la query', async () => {
  servicio(() => Response.json({ address: 'ana+news@x.eu' }));
  const alta = await confianza.POST(
    contexto(post('/api/trusted-senders', { address: 'Ana+News@X.eu' })),
  );
  expect(await alta.json()).toEqual({ address: 'ana+news@x.eu' });
  expect(llamadas[0]).toMatchObject({
    metodo: 'POST',
    ruta: '/api/v1/trusted-senders',
    cuerpo: { address: 'Ana+News@X.eu' },
  });

  servicio(() => new Response(null, { status: 204 }));
  const baja = await confianza.POST(
    contexto(post('/api/trusted-senders', { address: 'ana+news@x.eu', remove: true })),
  );
  expect(baja.status).toBe(200);
  // `+` sin codificar llegaría al servicio como un espacio.
  expect(llamadas[0]).toMatchObject({
    metodo: 'DELETE',
    ruta: '/api/v1/trusted-senders',
    query: '?address=ana%2Bnews%40x.eu',
  });
});

test('sin dirección no se llama al servicio; el tope del servicio se conserva', async () => {
  servicio(() => Response.json({}));
  const vacia = await confianza.POST(contexto(post('/api/trusted-senders', { address: ' ' })));
  expect(vacia.status).toBe(400);
  expect(llamadas).toHaveLength(0);

  servicio(() => Response.json({ error: { code: 'trusted_sender_limit' } }, { status: 409 }));
  const tope = await confianza.POST(contexto(post('/api/trusted-senders', { address: 'a@x.eu' })));
  expect(tope.status).toBe(409);
  expect(await tope.json()).toMatchObject({ code: 'trusted_sender_limit' });
});

test('GET de la lista de confianza deja fuera lo que no es texto', async () => {
  servicio(() => Response.json({ senders: ['a@x.eu', 3, null, 'b@y.eu'] }));
  const res = await confianza.GET(
    contexto(new Request('https://mail.sytue.eu/api/trusted-senders')),
  );
  expect(await res.json()).toEqual({ senders: ['a@x.eu', 'b@y.eu'] });
});
