/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { afterEach, expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import { rutasDeCompartir } from './compartir.js';
import type { Persona } from './directorio.js';
import type { ServerSession } from './app-bff.js';

const ana: Persona = { userId: 'sub-ana', email: 'ana@acme.test', name: 'Ana' };
const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

/** Lo que el servicio recibió, y lo que contesta. */
function servicio(responder: (url: string, init: RequestInit) => Response): string[] {
  const vistas: string[] = [];
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    vistas.push(`${init.method} ${url} ${typeof init.body === 'string' ? init.body : ''}`.trim());
    return responder(url, init);
  }) as typeof fetch;
  return vistas;
}

function rutas(conSesion = true) {
  return rutasDeCompartir({
    sesion: async () => (conSesion ? ({ accessToken: 'tok' } as ServerSession) : null),
    servicio: () => 'http://svc/api/v1',
    base: (q) => (q.get('album') ? `/albums/${q.get('album')}/people` : null),
    directorio: {
      resolverPersona: async (_ctx, email) => (email === ana.email ? ana : null),
      resolverPersonaPorId: async (_ctx, id) => (id === ana.userId ? ana : null),
      resolverPersonas: async () => new Map([[ana.userId, ana]]),
    },
  });
}

function ctx(url: string, init?: RequestInit): APIContext {
  return { request: new Request(`https://photos.test${url}`, init) } as unknown as APIContext;
}

test('compartir por correo guarda el sub que da Sytue ID, nunca el que manda el navegador', async () => {
  const vistas = servicio(() => Response.json({ created: true }));
  const res = await rutas().POST(
    ctx('/api/people?album=A1', {
      method: 'POST',
      body: JSON.stringify({ email: ana.email, subject: 'otro', permission: 'write' }),
    }),
  );
  expect(res.status).toBe(200);
  expect(vistas).toEqual(['PUT http://svc/api/v1/albums/A1/people/sub-ana {"permission":"write"}']);
  expect(await res.json()).toEqual({
    person: { subject: 'sub-ana', permission: 'write', email: ana.email, name: 'Ana' },
    created: true,
  });
});

test('un correo o un sub que no es de la organización no llega al servicio', async () => {
  const vistas = servicio(() => Response.json({}));
  for (const cuerpo of [{ email: 'nadie@fuera.test' }, { subject: 'sub-de-fuera' }]) {
    const res = await rutas().POST(
      ctx('/api/people?album=A1', { method: 'POST', body: JSON.stringify(cuerpo) }),
    );
    expect(res.status).toBe(404);
  }
  expect(vistas).toEqual([]);
});

test('sin sesión no se pregunta a nadie', async () => {
  const vistas = servicio(() => Response.json({}));
  const res = await rutas(false).POST(
    ctx('/api/people?album=A1', { method: 'POST', body: JSON.stringify({ email: ana.email }) }),
  );
  expect(res.status).toBe(401);
  expect(vistas).toEqual([]);
});

test('la lista vuelve con cara, y el estado del servicio se respeta', async () => {
  servicio(() =>
    Response.json({ people: [{ subject: 'sub-ana', permission: 'read', createdAt: 'x' }] }),
  );
  const res = await rutas().GET(ctx('/api/people?album=A1'));
  expect(await res.json()).toEqual({
    people: [{ subject: 'sub-ana', permission: 'read', email: ana.email, name: 'Ana' }],
  });
  servicio(() => new Response('{"error":{"code":"not_found"}}', { status: 404 }));
  expect((await rutas().GET(ctx('/api/people?album=ajeno'))).status).toBe(404);
});

test('dejar de compartir va por el sub de la lista', async () => {
  const vistas = servicio(() => new Response(null, { status: 204 }));
  const res = await rutas().DELETE(ctx('/api/people?album=A1&sub=sub-ana', { method: 'DELETE' }));
  expect(res.status).toBe(204);
  expect(vistas).toEqual(['DELETE http://svc/api/v1/albums/A1/people/sub-ana']);
  expect((await rutas().DELETE(ctx('/api/people?album=A1', { method: 'DELETE' }))).status).toBe(
    400,
  );
});
