import { afterEach, expect, test } from 'bun:test';
import type { OidcConfig } from './types.js';
import { buildAuthorizationRequest, exchangeCode, userFromClaims } from './oidc.js';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

test('usa la red interna solo para discovery y token, no para la redirección del navegador', async () => {
  const calls: string[] = [];
  const issuer = 'https://id.oidc-internal-test.sytue.eu';
  const config: OidcConfig = {
    issuer,
    internalBaseUrl: 'http://sytue-one-id',
    clientId: 'sytue-one',
    clientSecret: 'secret',
    redirectUri: 'https://one.sytue.eu/auth/callback',
    postLogoutRedirectUri: 'https://one.sytue.eu/',
  };

  globalThis.fetch = (async (input, init) => {
    const url = String(input);
    calls.push(url);
    if (url.endsWith('/.well-known/openid-configuration')) {
      return Response.json({
        issuer,
        authorization_endpoint: `${issuer}/oauth/authorize`,
        token_endpoint: `${issuer}/oauth/token`,
        userinfo_endpoint: `${issuer}/oauth/userinfo`,
        end_session_endpoint: `${issuer}/oauth/logout`,
        jwks_uri: `${issuer}/.well-known/jwks.json`,
      });
    }
    expect(init?.method).toBe('POST');
    return Response.json({
      access_token: 'access',
      token_type: 'Bearer',
      expires_in: 300,
    });
  }) as typeof fetch;

  const authorization = await buildAuthorizationRequest(config);
  expect(authorization.url.startsWith(`${issuer}/oauth/authorize?`)).toBe(true);
  expect(calls[0]).toBe('http://sytue-one-id/.well-known/openid-configuration');

  await exchangeCode(config, 'code', authorization.pkce.codeVerifier);
  expect(calls[1]).toBe('http://sytue-one-id/oauth/token');
});

test('rechaza discovery que intente sacar endpoints confidenciales fuera del issuer', async () => {
  const issuer = 'https://id.oidc-origin-test.sytue.eu';
  globalThis.fetch = (async () =>
    Response.json({
      issuer,
      authorization_endpoint: `${issuer}/oauth/authorize`,
      token_endpoint: 'https://evil.example/token',
      userinfo_endpoint: `${issuer}/oauth/userinfo`,
      end_session_endpoint: `${issuer}/oauth/logout`,
      jwks_uri: `${issuer}/.well-known/jwks.json`,
    })) as unknown as typeof fetch;

  await expect(
    buildAuthorizationRequest({
      issuer,
      clientId: 'sytue-one',
      clientSecret: 'secret',
      redirectUri: 'https://one.sytue.eu/auth/callback',
      postLogoutRedirectUri: 'https://one.sytue.eu/',
    }),
  ).rejects.toThrow('fuera del issuer');
});

// La organización activa sale del claim `tenant`, que es el que Sytue ID emite de
// verdad (services/id/internal/oidc/token.go). Antes se derivaba SOLO de un grupo
// `org/<slug>` que no emite nadie —`GroupsForUser` devuelve los nombres reales de
// la tabla `groups`, porque es lo que Drive matchea contra sus shares—, así que
// `user.tenant` era `undefined` en todas las sesiones de producción y con él se
// caía cualquier cosa colgada de la organización: el logo, su color, el menú de
// cuenta.
test('la organización sale del claim tenant, no de un grupo que nadie emite', () => {
  const claims = {
    sub: 'u-1',
    email: 'ana@acme.eu',
    tenant: 'acme',
    // Grupos REALES tal y como los emite el IdP: nombres legibles, sin prefijo.
    groups: ['Ingeniería', 'Dirección'],
    roles: ['admin'],
  };
  expect(userFromClaims(claims).tenant).toBe('acme');
});

test('sigue valiendo el grupo org/<slug> si algún día vuelve a emitirse', () => {
  expect(userFromClaims({ sub: 'u-2', groups: ['org/acme'] }).tenant).toBe('acme');
  expect(userFromClaims({ sub: 'u-3', groups: ['Ingeniería'] }).tenant).toBeUndefined();
});

test('el color de la organización solo entra si tiene forma de nombre de color', () => {
  expect(userFromClaims({ sub: 'u-4', org_color: 'cobalto' }).orgColor).toBe('cobalto');
  expect(userFromClaims({ sub: 'u-5' }).orgColor).toBeUndefined();
  // Acaba en una cookie que escribe el BFF: nada que no sean letras.
  expect(userFromClaims({ sub: 'u-6', org_color: 'x; Domain=evil' }).orgColor).toBeUndefined();
});

test('la forma de la organización entra por la misma puerta que el color', () => {
  expect(userFromClaims({ sub: 'u-7', org_forma: 'carril' }).orgForma).toBe('carril');
  expect(userFromClaims({ sub: 'u-8' }).orgForma).toBeUndefined();
  expect(userFromClaims({ sub: 'u-9', org_forma: 'x; Domain=evil' }).orgForma).toBeUndefined();
});

test('userFromClaims lee la clase de organización, el plan y la cuota', () => {
  const personal = userFromClaims({
    sub: 'u-9',
    tenant: 'p-0123456789abcdef',
    org_kind: 'personal',
    plan: 'personal_free',
    storage_bytes: 5 * 1024 * 1024 * 1024,
  });
  expect(personal.orgKind).toBe('personal');
  expect(personal.plan).toBe('personal_free');
  expect(personal.storageBytes).toBe(5 * 1024 * 1024 * 1024);
  // Un token anterior a la distinción no trae nada, y eso NO es «personal».
  const viejo = userFromClaims({ sub: 'u-10', tenant: 'acme' });
  expect(viejo.orgKind).toBeUndefined();
  expect(viejo.plan).toBeUndefined();
  // Y un valor desconocido no se cuela como clase.
  expect(userFromClaims({ sub: 'u-11', org_kind: 'weird' }).orgKind).toBeUndefined();
});

// El canje del código se reintenta UNA vez si la petición murió en la red (socket
// reutilizado que el servidor ya había cerrado). Una respuesta HTTP de error no es
// un fallo de red y no se reintenta.
function configDeCanje(): OidcConfig {
  return {
    issuer: 'https://id.canje-test.sytue.eu',
    internalBaseUrl: 'http://sytue-one-id',
    clientId: 'sytue-photos',
    clientSecret: 'secret',
    redirectUri: 'https://corporate.sytue.eu/photos/auth/callback',
    postLogoutRedirectUri: 'https://corporate.sytue.eu/photos/',
  };
}

function idpFalso(tokenRespuestas: Array<() => Promise<Response>>) {
  const issuer = 'https://id.canje-test.sytue.eu';
  let canjes = 0;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith('/.well-known/openid-configuration')) {
      return Response.json({
        issuer,
        authorization_endpoint: `${issuer}/oauth/authorize`,
        token_endpoint: `${issuer}/oauth/token`,
        userinfo_endpoint: `${issuer}/oauth/userinfo`,
        end_session_endpoint: `${issuer}/oauth/logout`,
        jwks_uri: `${issuer}/.well-known/jwks.json`,
      });
    }
    const respuesta = tokenRespuestas[canjes] ?? tokenRespuestas[tokenRespuestas.length - 1]!;
    canjes += 1;
    return respuesta();
  }) as typeof fetch;
  return () => canjes;
}

test('un socket muerto en el canje del código se reintenta una vez y el login sigue', async () => {
  const canjes = idpFalso([
    () => Promise.reject(new TypeError('The socket connection was closed unexpectedly')),
    async () => Response.json({ access_token: 'access', token_type: 'Bearer', expires_in: 300 }),
  ]);
  const tokens = await exchangeCode(configDeCanje(), 'code', 'verifier');
  expect(tokens.accessToken).toBe('access');
  expect(canjes()).toBe(2);
});

test('un error HTTP del canje NO se reintenta: no es la red', async () => {
  const canjes = idpFalso([async () => Response.json({ error: 'invalid_grant' }, { status: 400 })]);
  await expect(exchangeCode(configDeCanje(), 'code', 'verifier')).rejects.toThrow('400');
  expect(canjes()).toBe(1);
});

test('si la red falla dos veces, el error sale: un reintento, no un bucle', async () => {
  const canjes = idpFalso([() => Promise.reject(new TypeError('socket closed'))]);
  await expect(exchangeCode(configDeCanje(), 'code', 'verifier')).rejects.toThrow('socket closed');
  expect(canjes()).toBe(2);
});
