// El icono de una alternativa llega como SVG y se pinta como <img>. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';

// `gateway.ts` arrastra el wiring del BFF, que exige su configuración OIDC al
// cargarse. Se da aquí y se importa después, como en myorgs.test.ts.
process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);

const { toOption } = await import('./gateway.js');

test('el SVG del manifiesto se vuelve una imagen, nunca marcado que se inyecta', () => {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>';
  const o = toOption({ instance_id: 'i1', iconUrl: svg });
  expect(o?.iconUrl.startsWith('data:image/svg+xml,')).toBe(true);
  expect(o?.iconUrl).not.toContain('<script>');
  expect(toOption({ instance_id: 'i2' })?.iconUrl).toBe('');
});
