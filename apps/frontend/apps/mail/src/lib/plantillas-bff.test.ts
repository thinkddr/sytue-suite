/**
 * /api/templates — el BFF de las plantillas de respuesta (D-MAIL-06). Traduce el POST
 * del navegador al verbo del servicio (POST/PUT/DELETE), pasa la plantilla a camelCase
 * y conserva el estado y el `code` de los rechazos que la pantalla sabe contar (nombre
 * repetido, tope de plantillas), en vez de convertirlo todo en un 502.
 *
 * VISTO EN ROJO (2026-11-11): con el `rechazo` reducido a un 502 sin `code`, el tope
 * salía como «no se pudo guardar». Deshecho. Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () =>
    Promise.resolve({ accessToken: 't', user: { id: 'ada', name: 'Ada', email: 'ada@sytue.eu' } }),
}));

const { GET, POST } = await import('../pages/api/templates.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

const llamadas: { metodo: string; ruta: string; cuerpo: unknown }[] = [];

/** El servicio simulado: contesta `respuesta` a lo que se le pida y lo apunta. */
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

function contexto(request: Request): APIContext {
  return { request, url: new URL(request.url) } as unknown as APIContext;
}

const post = (cuerpo: unknown) =>
  POST(
    contexto(
      new Request('https://mail.sytue.eu/api/templates', {
        method: 'POST',
        body: JSON.stringify(cuerpo),
      }),
    ),
  );

const ID = '0b6f6c2e-6a39-4bd2-9a57-4f2d3c9f5a10';
const DEL_SERVICIO = {
  id: ID,
  name: 'Horario',
  body: 'De 9 a 14.',
  created_at: '2026-11-11T09:00:00Z',
  updated_at: '2026-11-11T10:00:00Z',
};

test('GET lista las plantillas en camelCase', async () => {
  servicio(() => Response.json({ templates: [DEL_SERVICIO] }));
  const res = await GET(contexto(new Request('https://mail.sytue.eu/api/templates')));
  expect(llamadas[0]).toMatchObject({ metodo: 'GET', ruta: '/api/v1/templates' });
  expect(await res.json()).toEqual({
    templates: [{ id: ID, name: 'Horario', body: 'De 9 a 14.', updatedAt: '2026-11-11T10:00:00Z' }],
  });
});

test('POST crea, cambia o borra según la forma del cuerpo', async () => {
  servicio(() => Response.json(DEL_SERVICIO, { status: 201 }));
  const creada = await post({ name: 'Horario', body: 'De 9 a 14.' });
  expect(creada.status).toBe(201);
  expect(llamadas[0]).toEqual({
    metodo: 'POST',
    ruta: '/api/v1/templates',
    cuerpo: { name: 'Horario', body: 'De 9 a 14.' },
  });

  servicio(() => Response.json(DEL_SERVICIO));
  await post({ id: ID, name: 'Horario', body: 'De 8 a 15.' });
  expect(llamadas[0]).toEqual({
    metodo: 'PUT',
    ruta: `/api/v1/templates/${ID}`,
    cuerpo: { name: 'Horario', body: 'De 8 a 15.' },
  });

  servicio(() => new Response(null, { status: 204 }));
  const borrada = await post({ id: ID, delete: true });
  expect(borrada.status).toBe(200);
  expect(llamadas[0]).toMatchObject({ metodo: 'DELETE', ruta: `/api/v1/templates/${ID}` });
});

test('un id que no es un uuid no llega al servicio', async () => {
  servicio(() => Response.json({}));
  const res = await post({ id: '../../mailboxes', delete: true });
  expect(res.status).toBe(400);
  expect(llamadas).toHaveLength(0);
});

test('el tope y el nombre repetido llegan con su estado y su code', async () => {
  for (const code of ['template_limit', 'template_exists']) {
    servicio(() => Response.json({ error: { code, message: 'x' } }, { status: 409 }));
    const res = await post({ name: 'Otra', body: '' });
    expect(res.status).toBe(409);
    expect(await res.json()).toMatchObject({ code });
  }
  // Un fallo del servicio sigue siendo un 502: no hay nada que la pantalla pueda explicar.
  servicio(() => new Response('boom', { status: 500 }));
  expect((await post({ name: 'Otra', body: '' })).status).toBe(502);
});
