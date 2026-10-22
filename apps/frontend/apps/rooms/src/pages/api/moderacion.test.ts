// Los BFF de denunciar y bloquear no hacen nada sin sesión: ni leen la cola ni
// llegan al servicio. Quién denuncia o bloquea lo pone el servicio a partir del
// token de la sesión; aquí se comprueba que sin ella no hay petición que valga.
process.env.SYTUE_OIDC_ISSUER = 'https://id.sytue.test';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-rooms';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'test-secret';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://rooms.sytue.test/auth/callback';
process.env.SYTUE_SESSION_SECRET = '01234567890123456789012345678901';

import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';

const reports = await import('./reports.js');
const blocks = await import('./blocks.js');

function sinSesion(method: string, body?: unknown): APIContext {
  return {
    params: {},
    request: new Request('http://localhost:4323/api/x', {
      method,
      ...(body === undefined
        ? {}
        : { headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) }),
    }),
    cookies: { get: () => undefined },
  } as unknown as APIContext;
}

test('sin sesión no se denuncia, no se bloquea y no se ve la cola', async () => {
  const respuestas = await Promise.all([
    reports.GET(sinSesion('GET')),
    reports.POST(sinSesion('POST', { messageId: 'm1', reason: 'spam' })),
    reports.POST(sinSesion('POST', { reportId: 'r1', action: 'dismissed' })),
    blocks.GET(sinSesion('GET')),
    blocks.POST(sinSesion('POST', { channelId: 'c1', subject: 'u2' })),
    blocks.POST(sinSesion('POST', { subject: 'u2', delete: true })),
  ]);
  expect(respuestas.map((r) => r.status)).toEqual([401, 401, 401, 401, 401, 401]);
});
