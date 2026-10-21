// El avatar de la barra de la suite: de dónde sale su URL y quién la sirve.
//
// Se rompió el 2026-09-24 en las trece superficies a la vez. `user.picture` era la
// URL que emite Sytue ID, `https://id.<dominio>/account/avatar?v=…`: en producción
// ese `/account` lo sirve la app «Mi cuenta», que no tiene esa página (404), y
// aunque llegara al IdP pedía SU cookie, que dura 12 h contra los 30 días de la de
// la app. El `<img>` salía roto junto a la campana.
//
// Ahora la URL es del MISMO origen que la página (`<base>/auth/avatar?v=…`) y la
// sirve el BFF con el token que ya tiene. Esto vigila las dos mitades, y que la
// ruta exista en TODAS las apps: una URL que apunta a una ruta que no existe es
// exactamente el fallo de antes. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import type { SytueSession } from '../types.js';
import { resolveOidcConfig } from '../config.js';
import { createBff } from './bff.js';
import { sealSession } from './cookies.js';

const SECRET = 'x'.repeat(32);
const APPS_DIR = join(import.meta.dir, '../../../../apps');

function bffDePrueba() {
  return createBff({
    config: resolveOidcConfig({
      SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
      SYTUE_OIDC_INTERNAL_BASE_URL: 'http://sytue-one-id',
      SYTUE_OIDC_CLIENT_ID: 'sytue-drive',
      SYTUE_OIDC_CLIENT_SECRET: 'secret',
      SYTUE_OIDC_REDIRECT_URI: 'https://corporate.sytue.eu/drive/auth/callback',
    }),
    sessionSecret: SECRET,
    basePath: '/drive',
  });
}

function sesionCon(picture?: string): SytueSession {
  return {
    clientId: 'sytue-drive',
    user: {
      id: 'u1',
      email: 'ada@sytue.eu',
      emailVerified: true,
      name: 'Ada Lovelace',
      preferredUsername: 'ada',
      picture,
    } as SytueSession['user'],
    // Lejos de caducar: si no, getSession intenta refrescar contra el IdP.
    tokens: { accessToken: 'tok', expiresAt: Date.now() + 3_600_000, tokenType: 'Bearer' },
  };
}

async function contexto(sesion: SytueSession, headers: Record<string, string> = {}) {
  const cookie = await sealSession(sesion, SECRET);
  const url = new URL('https://corporate.sytue.eu/drive/auth/avatar?v=abc');
  return {
    request: new Request(url, { headers }),
    url,
    cookies: { get: () => ({ value: cookie }), set: () => {}, delete: () => {} },
  };
}

/** Sustituye `fetch` durante la llamada y apunta qué se le pidió al IdP. */
async function conIdP<T>(respuesta: () => Response, f: () => Promise<T>) {
  const pedidas: { url: string; headers: Headers }[] = [];
  const real = globalThis.fetch;
  globalThis.fetch = ((url: string, init?: RequestInit) => {
    pedidas.push({ url: String(url), headers: new Headers(init?.headers) });
    return Promise.resolve(respuesta());
  }) as unknown as typeof fetch;
  try {
    return { out: await f(), pedidas };
  } finally {
    globalThis.fetch = real;
  }
}

// VISTO EN ROJO devolviendo `session` tal cual en getSession: `picture` sigue
// siendo la URL del IdP.
test('user.picture es del mismo origen que la página y conserva la versión', async () => {
  const bff = bffDePrueba();
  const s = await bff.getSession(
    await contexto(sesionCon('https://id.sytue.eu/account/avatar?v=lq3x9')),
  );
  expect(s?.user.picture).toBe('/drive/auth/avatar?v=lq3x9');
});

test('sin foto en Sytue ID no se inventa una URL: salen las iniciales', async () => {
  const s = await bffDePrueba().getSession(await contexto(sesionCon(undefined)));
  expect(s?.user.picture).toBeUndefined();
});

test('/auth/avatar pide la foto al IdP con el token y reenvía la revalidación', async () => {
  const bff = bffDePrueba();
  const ctx = await contexto(sesionCon('https://id.sytue.eu/account/avatar?v=1'), {
    'if-none-match': '"v1"',
  });
  const { out, pedidas } = await conIdP(
    () =>
      new Response(new Uint8Array([0x89, 0x50, 0x4e, 0x47]), {
        status: 200,
        headers: {
          'content-type': 'image/png',
          etag: '"v1"',
          'cache-control': 'private, max-age=3600, must-revalidate',
        },
      }),
    () => bff.avatar(ctx),
  );
  expect(pedidas).toHaveLength(1);
  expect(pedidas[0]?.url).toBe('http://sytue-one-id/api/v1/me/avatar');
  expect(pedidas[0]?.headers.get('authorization')).toBe('Bearer tok');
  expect(pedidas[0]?.headers.get('if-none-match')).toBe('"v1"');
  expect(out.status).toBe(200);
  expect(out.headers.get('content-type')).toBe('image/png');
  expect(out.headers.get('etag')).toBe('"v1"');
  expect(out.headers.get('cache-control')).toContain('private');
});

test('/auth/avatar relaya el 304 sin cuerpo y el 404 de «no tiene foto»', async () => {
  const bff = bffDePrueba();
  const ctx = await contexto(sesionCon('https://id.sytue.eu/account/avatar?v=1'));
  const no = await conIdP(
    () => new Response(null, { status: 304, headers: { etag: '"v1"' } }),
    () => bff.avatar(ctx),
  );
  expect(no.out.status).toBe(304);
  expect(no.out.headers.get('etag')).toBe('"v1"');
  const sin = await conIdP(
    () => new Response('{}', { status: 404 }),
    () => bff.avatar(ctx),
  );
  expect(sin.out.status).toBe(404);
  expect(sin.out.headers.get('cache-control')).toBe('no-store');
});

test('/auth/avatar sin sesión es 401 y no llama al IdP', async () => {
  const url = new URL('https://corporate.sytue.eu/drive/auth/avatar');
  const { out, pedidas } = await conIdP(
    () => new Response(null, { status: 200 }),
    () =>
      bffDePrueba().avatar({
        request: new Request(url),
        url,
        cookies: { get: () => undefined, set: () => {}, delete: () => {} },
      }),
  );
  expect(out.status).toBe(401);
  expect(pedidas).toHaveLength(0);
});

// Si una app no tiene la ruta, su barra pide una URL que da 404: el fallo de
// antes, en otra forma.
test('toda app con BFF sirve GET /auth/avatar', () => {
  const rutas = readdirSync(APPS_DIR, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => join(APPS_DIR, e.name, 'src/pages/auth'))
    .filter((dir) => {
      try {
        readdirSync(dir);
        return true;
      } catch {
        return false; // app sin sesión (www)
      }
    });
  expect(rutas.length).toBeGreaterThanOrEqual(13);
  for (const dir of rutas) {
    let src = '';
    try {
      src = readFileSync(join(dir, 'avatar.ts'), 'utf8');
    } catch {
      /* se queda vacío y falla abajo, nombrando la app */
    }
    expect({ dir, get: /\bavatar as GET\b/.test(src) }).toEqual({ dir, get: true });
  }
});
