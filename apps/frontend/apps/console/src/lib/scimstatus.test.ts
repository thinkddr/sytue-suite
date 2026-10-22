// El estado de SCIM que pinta la consola tiene que venir del servicio o no
// venir. Aquí hubo un KPI «Provisioning: OK» escrito en el código que decía que
// las bajas del directorio corporativo se aplicaban solas mientras nadie
// revocaba ninguna cuenta; la garantía que sustituye a aquello es que, sin
// respuesta, esto devuelve `live:false` y la tarjeta dice «se desconoce» en vez
// de inventarse un «no configurado». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { SytueSession } from '@sytue/auth';

process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);

const { loadScimStatus } = await import('./admin.js');

const sesion = { tokens: { accessToken: 'tok' } } as SytueSession;

function conRespuesta<T>(res: Response, fn: () => Promise<T>): Promise<T> {
  const real = globalThis.fetch;
  globalThis.fetch = (() => Promise.resolve(res)) as unknown as typeof fetch;
  return fn().finally(() => {
    globalThis.fetch = real;
  });
}

test('lee el estado medido, con la fecha ausente cuando no ha pasado nada', async () => {
  const { live, data } = await conRespuesta(
    new Response(
      JSON.stringify({
        configured: true,
        base_url: 'https://id.ejemplo/scim/v2',
        users_provisioned: 7,
      }),
      { status: 200, headers: { 'content-type': 'application/json' } },
    ),
    () => loadScimStatus(sesion),
  );
  expect(live).toBe(true);
  expect(data?.configured).toBe(true);
  expect(data?.usersProvisioned).toBe(7);
  expect(data?.baseUrl).toBe('https://id.ejemplo/scim/v2');
  expect(data?.lastEventAt).toBe('');
});

test('sin respuesta del servicio no se inventa «no configurado»', async () => {
  const { live, data } = await conRespuesta(new Response('<html>502</html>', { status: 502 }), () =>
    loadScimStatus(sesion),
  );
  expect(live).toBe(false);
  expect(data).toBeNull();
});
