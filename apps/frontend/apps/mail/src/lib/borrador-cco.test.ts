/**
 * Un borrador con CCO la conserva: guardarlo, cerrarlo y reabrirlo la enseña igual.
 *
 * Antes /api/drafts ni la recibía ni la devolvía, así que el primer autoguardado
 * la tiraba y el borrador reabierto se enviaba sin la copia oculta. Se prueba de
 * punta a punta del BFF —POST, el .eml que llega al servicio, y GET de ese mismo
 * .eml— con el SDK real y solo el servicio simulado.
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () =>
    Promise.resolve({
      accessToken: 't',
      user: { id: 'ada', name: 'Ada', email: 'ada@sytue.eu' },
    }),
}));
mock.module('./identidad.js', () => ({
  direccionDeEnvio: () => Promise.resolve('ada@sytue.eu'),
}));

const { GET, POST } = await import('../pages/api/drafts.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

/** Simula el servicio: guarda el .eml de cada borrador y lo devuelve en /raw. */
function servicioDeBorradores(): Map<string, string> {
  const guardados = new Map<string, string>();
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    const ruta = new URL(String(url)).pathname;
    if (ruta.endsWith('/.well-known/jmap')) {
      return Response.json({
        apiUrl: 'https://mail.test/jmap',
        downloadUrl: '',
        uploadUrl: '',
        primaryAccounts: { 'urn:ietf:params:jmap:mail': 'acc-1' },
        accounts: { 'acc-1': {} },
      });
    }
    if (ruta === '/api/v1/drafts' && init?.method === 'POST') {
      const id = `d-${guardados.size + 1}`;
      guardados.set(id, JSON.parse(String(init.body)).raw);
      return Response.json({ id }, { status: 201 });
    }
    const crudo = /^\/api\/v1\/messages\/([^/]+)\/raw$/.exec(ruta);
    if (crudo) return new Response(guardados.get(crudo[1]!) ?? '');
    return new Response('{}', { status: 404 });
  }) as unknown as typeof fetch;
  return guardados;
}

function contexto(request: Request): APIContext {
  return { request, url: new URL(request.url) } as unknown as APIContext;
}

test('guardar un borrador con CCO y reabrirlo devuelve la misma CCO', async () => {
  const guardados = servicioDeBorradores();
  const guardar = await POST(
    contexto(
      new Request('https://mail.sytue.eu/api/drafts', {
        method: 'POST',
        body: JSON.stringify({
          to: [{ email: 'bob@sytue.eu' }],
          bcc: [{ email: 'oculto@sytue.eu' }, { name: 'Eva Ruiz', email: 'eva@sytue.eu' }],
          subject: 'Con copia oculta',
          textBody: 'hola',
        }),
      }),
    ),
  );
  const { id } = (await guardar.json()) as { id: string };
  expect(id).toBe('d-1');
  expect(guardados.get('d-1')).toContain('\r\nBcc: oculto@sytue.eu, Eva Ruiz <eva@sytue.eu>\r\n');

  const reabrir = await GET(contexto(new Request(`https://mail.sytue.eu/api/drafts?id=${id}`)));
  const borrador = (await reabrir.json()) as { to: string; bcc: string };
  expect(borrador.to).toBe('bob@sytue.eu');
  expect(borrador.bcc).toBe('oculto@sytue.eu, Eva Ruiz <eva@sytue.eu>');
});

test('un borrador sin CCO no inventa la cabecera', async () => {
  const guardados = servicioDeBorradores();
  await POST(
    contexto(
      new Request('https://mail.sytue.eu/api/drafts', {
        method: 'POST',
        body: JSON.stringify({ to: [{ email: 'bob@sytue.eu' }], bcc: [], subject: 'x' }),
      }),
    ),
  );
  expect(guardados.get('d-1')?.toLowerCase()).not.toContain('bcc:');
});
