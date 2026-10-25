// Check del gate anti open-redirect del BFF (safeReturnTo): el returnTo que
// el callback OIDC emite como Location jamas puede salir del origen de la
// app. Verifica ademas la resolucion real con new URL, que replica como el
// navegador interpreta la cabecera ('\' como '/', \t \n \r eliminados).
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { resolveOidcConfig } from '../config.js';
import { hostCookieName, seal, sessionCookieAttributes, unseal } from './cookies.js';
import { conPrefijo, createBff, safeReturnTo } from './bff.js';
import { returnToCanonico } from '@sytue/config';

const BASE = 'https://mail.sytue.eu';

test('safeReturnTo: acepta rutas internas validas tal cual', () => {
  for (const p of ['/', '/app', '/mail/inbox?x=1', '/docs/d/abc#p2']) {
    expect(safeReturnTo(p)).toBe(p);
  }
});

test('safeReturnTo: rechaza vectores de open redirect', () => {
  const evil = [
    '//evil.com',
    '/\\evil.com', // el navegador trata '\' como '/' -> protocolo-relativa
    'https://evil.com',
    '\\\\evil.com',
    '/\t/evil.com', // el parser de URL elimina \t -> '//evil.com'
    '/\t\\evil.com',
    '/\n/evil.com',
    '/\r/evil.com',
    '',
    'evil.com',
  ];
  for (const p of evil) {
    expect(safeReturnTo(p)).toBe('/');
  }
});

test('safeReturnTo: lo emitido nunca resuelve fuera del origen', () => {
  const inputs = [
    '/app',
    '/mail/inbox?x=1',
    '//evil.com',
    '/\\evil.com',
    '/\t/evil.com',
    'https://evil.com',
  ];
  for (const p of inputs) {
    expect(new URL(safeReturnTo(p), BASE).origin).toBe(BASE);
  }
});

test('las cookies de producción son __Host- y nunca llevan Domain', () => {
  expect(hostCookieName('sytue_session', true)).toBe('__Host-sytue_session');
  expect(hostCookieName('sytue_session', false)).toBe('sytue_session');
  expect(sessionCookieAttributes({ maxAge: 60, secure: true })).toEqual({
    path: '/',
    httpOnly: true,
    secure: true,
    sameSite: 'lax',
    maxAge: 60,
  });
});

test('rechaza una configuración heredada que comparta cookies con el apex', () => {
  expect(() =>
    resolveOidcConfig({
      SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
      SYTUE_OIDC_CLIENT_ID: 'sytue-mail',
      SYTUE_OIDC_CLIENT_SECRET: 'secret',
      SYTUE_OIDC_REDIRECT_URI: 'https://mail.sytue.eu/auth/callback',
      SYTUE_COOKIE_DOMAIN: '.sytue.eu',
    }),
  ).toThrow('host-only');
});

test('separa el issuer público del origen OIDC interno del BFF', () => {
  const config = resolveOidcConfig({
    SYTUE_OIDC_ISSUER: 'https://id.sytue.eu/',
    SYTUE_OIDC_INTERNAL_BASE_URL: 'http://sytue-one-id/',
    SYTUE_OIDC_CLIENT_ID: 'sytue-mail',
    SYTUE_OIDC_CLIENT_SECRET: 'secret',
    SYTUE_OIDC_REDIRECT_URI: 'https://mail.sytue.eu/auth/callback',
  });
  expect(config.issuer).toBe('https://id.sytue.eu');
  expect(config.internalBaseUrl).toBe('http://sytue-one-id');
});

test('rechaza un origen OIDC interno con ruta o credenciales', () => {
  const base = {
    SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
    SYTUE_OIDC_CLIENT_ID: 'sytue-mail',
    SYTUE_OIDC_CLIENT_SECRET: 'secret',
    SYTUE_OIDC_REDIRECT_URI: 'https://mail.sytue.eu/auth/callback',
  };
  expect(() =>
    resolveOidcConfig({
      ...base,
      SYTUE_OIDC_INTERNAL_BASE_URL: 'http://user:pass@sytue-one-id/private',
    }),
  ).toThrow('origen sin credenciales');
});

// `roles` NO es un scope de Sytue ID: es un claim que el IdP emite siempre que el
// usuario tiene roles en el tenant. Pedirlo devolvía invalid_scope en authorize y
// el BFF reintentaba el login hasta ERR_TOO_MANY_REDIRECTS (incidente 2026-08-03).
// La lista viva es scopes_supported en services/id/internal/oidc/discovery.go.
const SCOPES_SOPORTADOS = [
  'openid',
  'profile',
  'email',
  'offline_access',
  'admin',
  'hub.read',
  'cloud.deploy',
  'cloud.read',
];

test('los scopes por defecto existen todos en Sytue ID', () => {
  const config = resolveOidcConfig({
    SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
    SYTUE_OIDC_CLIENT_ID: 'sytue-console',
    SYTUE_OIDC_CLIENT_SECRET: 'secret',
    SYTUE_OIDC_REDIRECT_URI: 'https://admin.sytue.eu/auth/callback',
  });
  expect(config.scopes).not.toContain('roles');
  for (const scope of config.scopes ?? []) {
    expect(SCOPES_SOPORTADOS).toContain(scope);
  }
});

test('un error del authorization endpoint NO redirige: corta el bucle', async () => {
  const bff = createBff({
    config: resolveOidcConfig({
      SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
      SYTUE_OIDC_CLIENT_ID: 'sytue-console',
      SYTUE_OIDC_CLIENT_SECRET: 'secret',
      SYTUE_OIDC_REDIRECT_URI: 'https://admin.sytue.eu/auth/callback',
    }),
    sessionSecret: 'x'.repeat(32),
  });
  const url = new URL(
    'https://admin.sytue.eu/auth/callback?error=invalid_scope&error_description=scope+no+permitido%3A+roles&state=abc',
  );
  const res = await bff.callback({
    request: new Request(url),
    url,
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  });
  expect(res.status).toBe(502);
  expect(res.headers.get('location')).toBeNull();
  const body = await res.text();
  expect(body).toContain('invalid_scope');
  expect(body).toContain('scope no permitido: roles');
});

// Cierre de sesión LOCAL (`?scope=local`): lo que permite que salir de una app
// cierre las diez. Cada BFF emite una cookie host-only, así que solo su propio
// host puede borrarla, y se lo pide Sytue ID al cerrar el SSO.
function bffDeConsola() {
  return createBff({
    config: resolveOidcConfig({
      SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
      SYTUE_OIDC_CLIENT_ID: 'sytue-console',
      SYTUE_OIDC_CLIENT_SECRET: 'secret',
      SYTUE_OIDC_REDIRECT_URI: 'https://admin.sytue.eu/auth/callback',
    }),
    sessionSecret: 'x'.repeat(32),
  });
}

async function logoutLocal(origin: string | null) {
  const borradas: Array<{ name: string; options?: Record<string, unknown> }> = [];
  const url = new URL('https://admin.sytue.eu/auth/logout?scope=local');
  const res = await bffDeConsola().logout({
    request: new Request(url, {
      method: 'POST',
      headers: origin === null ? {} : { origin },
    }),
    url,
    cookies: {
      get: () => undefined,
      set: () => {},
      delete: (name: string, options?: Record<string, unknown>) =>
        // Con `exactOptionalPropertyTypes`, pasar `options: undefined` NO es lo mismo que
        // no pasar la clave; se omite cuando no viene.
        borradas.push(options === undefined ? { name } : { name, options }),
    },
  });
  return { res, borradas };
}

test('el cierre local borra la cookie y no arrastra al IdP', async () => {
  const { res, borradas } = await logoutLocal('https://mail.sytue.eu');
  expect(res.status).toBe(204);
  // Sin Location: hay una sola sesión SSO y la cierra la navegación final.
  expect(res.headers.get('location')).toBeNull();
  expect(borradas.map((b) => b.name)).toContain('__Host-sytue_session');
});

test('el borrado de una cookie __Host- lleva Secure y Path=/', async () => {
  // El navegador RECHAZA entero cualquier Set-Cookie de un nombre `__Host-` que
  // no cumpla el prefijo (Secure + Path=/ + sin Domain), y eso incluye el de
  // borrado: sin `secure`, cerrar sesión emitía la cabecera, el navegador la
  // descartaba en silencio y la sesión seguía viva en las diez superficies.
  const { borradas } = await logoutLocal(null);
  const host = borradas.filter((b) => b.name.startsWith('__Host-'));
  expect(host.length).toBeGreaterThan(0);
  for (const b of host) {
    expect(b.options).toMatchObject({ path: '/', secure: true });
    expect(b.options?.domain).toBeUndefined();
  }
});

test('el cierre local no concede CORS a NINGUN origen', async () => {
  // El barrido lo pide Sytue ID cargando esta URL como subrecurso, que no
  // necesita CORS: la cookie viaja igual (misma suite = mismo site) y el
  // Set-Cookie de borrado se aplica. Conceder la cabecera solo abriria la
  // respuesta a lectura desde otro origen sin que nadie la necesite.
  for (const origin of ['https://mail.sytue.eu', 'https://sytue.eu.evil.com', null]) {
    const { res } = await logoutLocal(origin);
    expect(res.status).toBe(204);
    expect(res.headers.get('access-control-allow-origin')).toBeNull();
  }
});

// El destino sobrevive a un callback que cae en OTRO host.
//
// Es el caso de todos los días en producción: `docs.sytue.eu/docs/edit?id=X`
// arranca el login en `docs.`, pero el `redirect_uri` del cliente apunta al
// dominio de la organización, así que el callback se ejecuta donde la cookie de
// estado —host-only— no existe. Ahí el flujo se reinicia, y antes se reiniciaba
// SIN saber a dónde iba: el enlace acababa en la lista de documentos, con el
// `?id=` perdido. Ahora el destino viaja sellado en el `state`.
function bffDeDocs() {
  return createBff({
    config: resolveOidcConfig({
      SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
      SYTUE_OIDC_CLIENT_ID: 'sytue-docs',
      SYTUE_OIDC_CLIENT_SECRET: 'secret',
      SYTUE_OIDC_REDIRECT_URI: 'https://corporate.sytue.eu/docs/auth/callback',
    }),
    sessionSecret: 'x'.repeat(32),
    basePath: '/docs',
  });
}

test('el callback sin cookie de estado reinicia el login CON el destino', async () => {
  const state = await seal({ s: 'aleatorio', r: '/docs/edit?id=X' }, 'x'.repeat(32));
  const url = new URL(
    `https://corporate.sytue.eu/docs/auth/callback?code=c&state=${encodeURIComponent(state)}`,
  );
  const res = await bffDeDocs().callback({
    request: new Request(url),
    url,
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  });
  expect(res.headers.get('location')).toBe(
    `/docs/auth/login?returnTo=${encodeURIComponent('/docs/edit?id=X')}`,
  );
});

test('un destino externo colado en el state no sale del origen', async () => {
  const state = await seal({ s: 'aleatorio', r: 'https://evil.com' }, 'x'.repeat(32));
  const url = new URL(
    `https://corporate.sytue.eu/docs/auth/callback?code=c&state=${encodeURIComponent(state)}`,
  );
  const res = await bffDeDocs().callback({
    request: new Request(url),
    url,
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  });
  // safeReturnTo lo reduce a '/', y entonces no se propaga returnTo ninguno.
  expect(res.headers.get('location')).toBe('/docs/auth/login');
});

test('un state plano (flujo empezado antes del cambio) sigue reiniciando el login', async () => {
  const url = new URL('https://corporate.sytue.eu/docs/auth/callback?code=c&state=abc');
  const res = await bffDeDocs().callback({
    request: new Request(url),
    url,
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  });
  expect(res.headers.get('location')).toBe('/docs/auth/login');
});

// El login empieza en el host donde va a terminar. Antes, entrar por el host del
// producto hacía DOS vueltas enteras por el IdP: la cookie de estado nacía en
// `docs.sytue.eu` y el callback corría en `corporate.sytue.eu`, donde no existe.
function cookiesQueApuntan() {
  const puestas: string[] = [];
  return {
    puestas,
    cookies: { get: () => undefined, set: (n: string) => void puestas.push(n), delete: () => {} },
  };
}

async function conDiscovery<T>(f: () => Promise<T>): Promise<T> {
  const original = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({
        issuer: 'https://id.sytue.eu',
        authorization_endpoint: 'https://id.sytue.eu/oauth2/authorize',
        token_endpoint: 'https://id.sytue.eu/oauth2/token',
        userinfo_endpoint: 'https://id.sytue.eu/oauth2/userinfo',
        end_session_endpoint: 'https://id.sytue.eu/oauth2/logout',
        jwks_uri: 'https://id.sytue.eu/.well-known/jwks.json',
      }),
      { headers: { 'content-type': 'application/json' } },
    )) as unknown as typeof fetch;
  try {
    return await f();
  } finally {
    globalThis.fetch = original;
  }
}

test('el login en el host del producto salta al del callback SIN crear la cookie de estado', async () => {
  const url = new URL(
    `https://docs.sytue.eu/docs/auth/login?returnTo=${encodeURIComponent('/docs/edit?id=X')}`,
  );
  const { puestas, cookies } = cookiesQueApuntan();
  const res = await bffDeDocs().login({ request: new Request(url), url, cookies });
  expect(res.status).toBe(302);
  expect(res.headers.get('location')).toBe(
    `https://corporate.sytue.eu/docs/auth/login?returnTo=${encodeURIComponent('/docs/edit?id=X')}`,
  );
  // Una cookie de estado en el host equivocado es justo lo que causaba la segunda vuelta.
  expect(puestas).toEqual([]);
});

test('el login en el host del callback va directo al IdP con la cookie de estado', async () => {
  const url = new URL('https://corporate.sytue.eu/docs/auth/login?returnTo=%2Fdocs%2F');
  const { puestas, cookies } = cookiesQueApuntan();
  const res = await conDiscovery(() =>
    bffDeDocs().login({ request: new Request(url), url, cookies }),
  );
  expect(res.status).toBe(302);
  const location = new URL(res.headers.get('location') ?? '');
  expect(`${location.origin}${location.pathname}`).toBe('https://id.sytue.eu/oauth2/authorize');
  expect(location.searchParams.get('redirect_uri')).toBe(
    'https://corporate.sytue.eu/docs/auth/callback',
  );
  expect(puestas).toEqual([hostCookieName('sytue_oidc_state', true)]);
});

test('reauth=1 pide a Sytue ID autenticar de nuevo (max_age=0); sin él, no', async () => {
  // Confirmar la identidad tras un 401 insufficient_user_authentication (step-up).
  for (const [query, maxAge] of [
    ['?reauth=1&returnTo=%2Fdocs%2F', '0'],
    ['?returnTo=%2Fdocs%2F', null],
  ] as const) {
    const url = new URL(`https://corporate.sytue.eu/docs/auth/login${query}`);
    const res = await conDiscovery(() =>
      bffDeDocs().login({ request: new Request(url), url, cookies: cookiesQueApuntan().cookies }),
    );
    expect(new URL(res.headers.get('location') ?? '').searchParams.get('max_age')).toBe(maxAge);
  }
});

test('con el origen de respaldo de Astro (http://localhost) NO salta: sería un bucle', async () => {
  // Si el host de la petición no es de confianza, Astro deja `ctx.url` en
  // http://localhost:<puerto>. Redirigir al host del callback desde ahí volvería a
  // caer en lo mismo en cada vuelta.
  const url = new URL('http://localhost:4321/docs/auth/login');
  const { cookies } = cookiesQueApuntan();
  const res = await conDiscovery(() =>
    bffDeDocs().login({ request: new Request(url), url, cookies }),
  );
  expect(new URL(res.headers.get('location') ?? '').host).toBe('id.sytue.eu');
});

// EL DESTINO LLEVA EL PREFIJO DE LA APP, lo traiga o no. En su host propio la app
// atiende también `/` y las páginas construyen el `returnTo` con `url.pathname`, así
// que llegaba `/?id=X`; resuelto en el host de la organización, eso es el Hub.
// Medido el 2026-09-25 con el enlace de una reunión creada desde Calendar.
test('el login guarda el destino con el prefijo de la app aunque llegue sin él', async () => {
  const url = new URL(
    `https://corporate.sytue.eu/docs/auth/login?returnTo=${encodeURIComponent('/?id=X')}`,
  );
  const res = await conDiscovery(() =>
    bffDeDocs().login({ request: new Request(url), url, cookies: cookiesQueApuntan().cookies }),
  );
  const state = new URL(res.headers.get('location') ?? '').searchParams.get('state') ?? '';
  const externo = await unseal<{ r: string }>(state, 'x'.repeat(32));
  expect(externo?.r).toBe('/docs/?id=X');
});

test('Mi cuenta conserva el aviso MFA en la vuelta OIDC sellada', async () => {
  const page = new URL('https://id.sytue.eu/account?mfa=required');
  const returnTo = returnToCanonico(page, '/account');
  const url = new URL(
    `https://id.sytue.eu/account/auth/login?returnTo=${encodeURIComponent(returnTo)}`,
  );
  const bff = createBff({
    config: resolveOidcConfig({
      SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
      SYTUE_OIDC_CLIENT_ID: 'sytue-account',
      SYTUE_OIDC_CLIENT_SECRET: 'secret',
      SYTUE_OIDC_REDIRECT_URI: 'https://id.sytue.eu/account/auth/callback',
    }),
    sessionSecret: 'x'.repeat(32),
    basePath: '/account',
  });
  let sealedCookie = '';
  const response = await conDiscovery(() =>
    bff.login({
      request: new Request(url),
      url,
      cookies: {
        get: () => undefined,
        set: (_name, value) => void (sealedCookie = value),
        delete: () => {},
      },
    }),
  );
  expect(response.status).toBe(302);
  const authorize = new URL(response.headers.get('location') ?? '');
  expect(authorize.searchParams.get('client_id')).toBe('sytue-account');
  const cookie = await unseal<{ returnTo: string }>(sealedCookie, 'x'.repeat(32));
  const state = await unseal<{ r: string }>(
    authorize.searchParams.get('state') ?? '',
    'x'.repeat(32),
  );
  expect(cookie?.returnTo).toBe('/account?mfa=required');
  expect(state?.r).toBe('/account?mfa=required');
  expect(safeReturnTo(state?.r ?? '')).toBe('/account?mfa=required');
});

test('conPrefijo: idempotente, respeta el límite de segmento y no toca lo que no es ruta', () => {
  const casos: [string, string][] = [
    ['/', '/rooms/'],
    ['/?rooms=1', '/rooms/?rooms=1'],
    ['/rooms', '/rooms'],
    ['/rooms/', '/rooms/'],
    ['/rooms?rooms=1', '/rooms?rooms=1'],
    ['/rooms/channels/a', '/rooms/channels/a'],
    ['/roomsx', '/rooms/roomsx'],
    ['https://evil.com', 'https://evil.com'],
    ['//evil.com', '//evil.com'],
  ];
  for (const [entrada, salida] of casos) expect(conPrefijo('/rooms', entrada)).toBe(salida);
  expect(conPrefijo('', '/?x=1')).toBe('/?x=1');
});

// Un canje fallido NO puede escapar como excepción: Astro la sirve como un 500
// sin Content-Type y Safari la DESCARGA (Cast, 2026-09-28, `invalid_client`).
async function callbackConCanje(estado: number, error: string) {
  const secreto = 'x'.repeat(32);
  const cookie = await seal(
    { state: 'aleatorio', nonce: 'n', codeVerifier: 'v', returnTo: '/docs/' },
    secreto,
  );
  const url = new URL('https://corporate.sytue.eu/docs/auth/callback?code=c&state=aleatorio');
  const original = globalThis.fetch;
  globalThis.fetch = (async (entrada: string | URL | Request) =>
    String(entrada instanceof Request ? entrada.url : entrada).endsWith('/oauth2/token')
      ? new Response(JSON.stringify({ error }), {
          status: estado,
          headers: { 'content-type': 'application/json' },
        })
      : new Response(
          JSON.stringify({
            issuer: 'https://id.sytue.eu',
            authorization_endpoint: 'https://id.sytue.eu/oauth2/authorize',
            token_endpoint: 'https://id.sytue.eu/oauth2/token',
            end_session_endpoint: 'https://id.sytue.eu/oauth2/logout',
            jwks_uri: 'https://id.sytue.eu/.well-known/jwks.json',
          }),
          { headers: { 'content-type': 'application/json' } },
        )) as unknown as typeof fetch;
  try {
    return await bffDeDocs().callback({
      request: new Request(url),
      url,
      cookies: { get: () => ({ value: cookie }), set: () => {}, delete: () => {} },
    });
  } finally {
    globalThis.fetch = original;
  }
}

test('un canje rechazado (invalid_client) es una página HTML, no una excepción', async () => {
  const res = await callbackConCanje(401, 'invalid_client');
  expect(res.status).toBe(502);
  expect(res.headers.get('content-type')).toStartWith('text/html');
  expect(await res.text()).toContain('invalid_client');
});

test('un code ya canjeado (invalid_grant, recargar o Atrás) reinicia el login', async () => {
  const res = await callbackConCanje(400, 'invalid_grant');
  expect(res.headers.get('location')).toBe('/docs/auth/login');
});

test('la página de error escapa lo que devuelve el IdP', async () => {
  const url = new URL(
    'https://admin.sytue.eu/auth/callback?error=x&error_description=%3Cscript%3Ealert(1)%3C%2Fscript%3E',
  );
  const res = await bffDeConsola().callback({
    request: new Request(url),
    url,
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  });
  expect(await res.text()).not.toContain('<script>');
});

// El color de la organización viaja en el id_token y el script del primer pintado
// no ve la sesión —va sellada y es HttpOnly—, así que el BFF lo deja al lado, en
// una cookie que sí se lee. Tiene que seguir a la sesión en CADA escritura: si el
// administrador vuelve al color de la casa, la cookie se borra, no se queda.
async function refrescarCon(orgColor: string | undefined, orgForma?: string) {
  const puestas: Array<{ name: string; value: string; options?: Record<string, unknown> }> = [];
  const borradas: string[] = [];
  const sesion = await seal(
    {
      user: {
        id: 'u-1',
        tenant: 'acme',
        ...(orgColor ? { orgColor } : {}),
        ...(orgForma ? { orgForma } : {}),
      },
      tokens: { accessToken: 'a', refreshToken: 'r', expiresAt: Date.now() + 300_000 },
      clientId: 'sytue-docs',
    },
    'x'.repeat(32),
  );
  const url = new URL('https://corporate.sytue.eu/docs/auth/refresh');
  const original = globalThis.fetch;
  globalThis.fetch = (async (entrada: string | URL | Request) =>
    new Response(
      JSON.stringify(
        String(entrada instanceof Request ? entrada.url : entrada).endsWith('/oauth2/token')
          ? { access_token: 'a2', refresh_token: 'r2', expires_in: 300 }
          : {
              issuer: 'https://id.sytue.eu',
              authorization_endpoint: 'https://id.sytue.eu/oauth2/authorize',
              token_endpoint: 'https://id.sytue.eu/oauth2/token',
              end_session_endpoint: 'https://id.sytue.eu/oauth2/logout',
              jwks_uri: 'https://id.sytue.eu/.well-known/jwks.json',
            },
      ),
      { headers: { 'content-type': 'application/json' } },
    )) as unknown as typeof fetch;
  try {
    const res = await bffDeDocs().refresh({
      request: new Request(url, { method: 'POST' }),
      url,
      cookies: {
        get: () => ({ value: sesion }),
        set: (name: string, value: string, options?: Record<string, unknown>) =>
          void puestas.push(options === undefined ? { name, value } : { name, value, options }),
        delete: (name: string) => void borradas.push(name),
      },
    });
    expect(res.status).toBe(200);
  } finally {
    globalThis.fetch = original;
  }
  return { puestas, borradas };
}

test('la sesión deja a su lado el color de la organización, legible y host-only', async () => {
  const { puestas, borradas } = await refrescarCon('cobalto');
  expect(puestas.map((c) => c.name)).toEqual(['__Host-sytue_session', 'sytue-color-org']);
  const color = puestas[1];
  expect(color?.value).toBe('cobalto');
  expect(color?.options).toMatchObject({ path: '/', secure: true, httpOnly: false });
  expect(color?.options?.domain).toBeUndefined();
  expect(borradas).toEqual(['sytue-forma-org']);
});

test('y la forma de la organización, por el mismo camino', async () => {
  const { puestas, borradas } = await refrescarCon(undefined, 'carril');
  expect(puestas.map((c) => c.name)).toEqual(['__Host-sytue_session', 'sytue-forma-org']);
  expect(puestas[1]?.value).toBe('carril');
  expect(puestas[1]?.options).toMatchObject({ path: '/', secure: true, httpOnly: false });
  expect(borradas).toEqual(['sytue-color-org']);
});

test('una organización con el color de la casa no deja cookie de color', async () => {
  const { puestas, borradas } = await refrescarCon(undefined);
  expect(puestas.map((c) => c.name)).toEqual(['__Host-sytue_session']);
  expect(borradas).toEqual(['sytue-color-org', 'sytue-forma-org']);
});
