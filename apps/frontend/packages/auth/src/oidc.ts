/**
 * Cliente OIDC de bajo nivel contra Sytue ID (servicio Go propio, id.sytue.eu):
 * discovery (/.well-known/openid-configuration), intercambio de codigo
 * (Authorization Code + PKCE), refresh, userinfo y verificacion del id_token
 * (jose). Solo se usa en el servidor (BFF). SIN rutas /realms/ (Keycloak fuera).
 */
import { createRemoteJWKSet, jwtVerify, type JWTPayload } from 'jose';
import type { DiscoveryDocument, OidcConfig, SytueRole, SytueUser, TokenSet } from './types.js';
import { createPkcePair, randomToken, type PkcePair } from './pkce.js';

const discoveryCache = new Map<string, DiscoveryDocument>();
const jwksCache = new Map<string, ReturnType<typeof createRemoteJWKSet>>();

/** Documento de descubrimiento OIDC del issuer (cacheado en memoria). */
export async function discover(
  issuer: string,
  internalBaseUrl?: string,
): Promise<DiscoveryDocument> {
  const cacheKey = `${issuer}\n${internalBaseUrl ?? ''}`;
  const cached = discoveryCache.get(cacheKey);
  if (cached) return cached;
  const discoveryUrl = new URL('/.well-known/openid-configuration', internalBaseUrl ?? issuer);
  const res = await fetch(discoveryUrl);
  if (!res.ok) throw new Error(`OIDC discovery fallo (${res.status}) para ${issuer}.`);
  const doc = (await res.json()) as DiscoveryDocument;
  if (doc.issuer?.replace(/\/$/, '') !== issuer.replace(/\/$/, '')) {
    throw new Error(`OIDC discovery devolvió un issuer distinto para ${issuer}.`);
  }
  for (const endpoint of [
    doc.authorization_endpoint,
    doc.token_endpoint,
    doc.userinfo_endpoint,
    doc.end_session_endpoint,
    doc.jwks_uri,
  ]) {
    const parsed = new URL(endpoint);
    if (parsed.origin !== new URL(issuer).origin) {
      throw new Error(`OIDC discovery devolvió un endpoint fuera del issuer ${issuer}.`);
    }
  }
  discoveryCache.set(cacheKey, doc);
  return doc;
}

/**
 * Traduce un endpoint público publicado por discovery a la ruta privada del
 * Service de Sytue ID. Se conserva path/query y jamás se acepta que discovery
 * desvíe una llamada confidencial a otro origen.
 */
function internalEndpoint(config: OidcConfig, endpoint: string): string {
  const publicUrl = new URL(endpoint);
  if (publicUrl.origin !== new URL(config.issuer).origin) {
    throw new Error(`OIDC: endpoint fuera del issuer ${config.issuer}.`);
  }
  if (!config.internalBaseUrl) return publicUrl.toString();
  const internalUrl = new URL(config.internalBaseUrl);
  internalUrl.pathname = publicUrl.pathname;
  internalUrl.search = publicUrl.search;
  internalUrl.hash = '';
  return internalUrl.toString();
}

export interface AuthorizationRequest {
  url: string;
  state: string;
  nonce: string;
  pkce: PkcePair;
}

/** Construye la URL de autorizacion (paso 1 del Authorization Code + PKCE). */
export async function buildAuthorizationRequest(
  config: OidcConfig,
  options?: { prompt?: 'login' | 'consent' | 'none'; loginHint?: string },
): Promise<AuthorizationRequest> {
  const doc = await discover(config.issuer, config.internalBaseUrl);
  const pkce = await createPkcePair();
  const state = randomToken();
  const nonce = randomToken();
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: config.clientId,
    redirect_uri: config.redirectUri,
    scope: (config.scopes ?? ['openid', 'profile', 'email']).join(' '),
    state,
    nonce,
    code_challenge: pkce.codeChallenge,
    code_challenge_method: pkce.codeChallengeMethod,
  });
  if (options?.prompt) params.set('prompt', options.prompt);
  if (options?.loginHint) params.set('login_hint', options.loginHint);
  return { url: `${doc.authorization_endpoint}?${params.toString()}`, state, nonce, pkce };
}

/**
 * El código de error OAuth de una respuesta fallida del token endpoint, listo
 * para pegar al mensaje. Cadena vacía si no hay ninguno legible.
 *
 * POR QUÉ. «Intercambio de código falló (400)» a secas no distingue el caso
 * benigno del grave, y los dos salen igual en los registros: `invalid_grant` es
 * casi siempre alguien que recargó la URL de retorno o pulsó Atrás —el código de
 * autorización es de un solo uso—, mientras que `invalid_client` significa que
 * nuestro client_id o client_secret están mal y NADIE puede entrar en esa app.
 * Auditando producción apareció uno de estos y no había forma de saber cuál.
 *
 * Va solo el campo `error`, que es un enumerado fijo del RFC 6749. NO va
 * `error_description`: es texto libre del servidor y puede llevar dentro lo que
 * mandó el cliente.
 */
async function motivoOAuth(res: Response): Promise<string> {
  try {
    const json = (await res.clone().json()) as Record<string, unknown>;
    const error = json.error;
    return typeof error === 'string' && error ? `, ${error}` : '';
  } catch {
    // Un cuerpo que no es JSON no dice nada útil y no puede tumbar el mensaje
    // de error: se pierde el matiz, no el aviso.
    return '';
  }
}

function parseTokenResponse(json: Record<string, unknown>): TokenSet {
  const expiresIn = typeof json.expires_in === 'number' ? json.expires_in : 300;
  return {
    accessToken: String(json.access_token),
    refreshToken: typeof json.refresh_token === 'string' ? json.refresh_token : undefined,
    idToken: typeof json.id_token === 'string' ? json.id_token : undefined,
    expiresAt: Date.now() + expiresIn * 1000,
    scope: typeof json.scope === 'string' ? json.scope : undefined,
    tokenType: typeof json.token_type === 'string' ? json.token_type : 'Bearer',
  };
}

/** Intercambia el `code` por tokens (paso 2). */
export async function exchangeCode(
  config: OidcConfig,
  code: string,
  codeVerifier: string,
): Promise<TokenSet> {
  const doc = await discover(config.issuer, config.internalBaseUrl);
  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    code,
    redirect_uri: config.redirectUri,
    client_id: config.clientId,
    client_secret: config.clientSecret,
    code_verifier: codeVerifier,
  });
  const canjear = () =>
    fetch(internalEndpoint(config, doc.token_endpoint), {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body,
    });
  // UN reintento si la petición no llegó a tener respuesta: `fetch` solo rechaza por
  // red, nunca por un estado HTTP. En el clúster el BFF reutiliza conexiones con
  // Sytue ID y el servidor Go cierra las inactivas a los 60 s; si coinciden, la
  // petición sale por un socket muerto («The socket connection was closed
  // unexpectedly») y el callback devolvía un 500 en pleno login. Medido en Fotos el
  // 2026-09-14, con el despliegue ya asentado.
  //
  // Reintentar el CANJE es seguro porque el código es de un solo uso: si la primera
  // petición sí llegó a canjearlo, la segunda falla con `invalid_grant`, que es el
  // mismo fallo que había. `refreshTokens` NO se reintenta así, a propósito: si la
  // primera ya rotó el refresh token, reusar el viejo puede hacer que el IdP revoque
  // toda la familia de tokens.
  const res = await canjear().catch(() => canjear());
  if (!res.ok)
    throw new Error(`Intercambio de código falló (${res.status}${await motivoOAuth(res)}).`);
  return parseTokenResponse((await res.json()) as Record<string, unknown>);
}

/** Refresca los tokens usando el refresh_token. */
export async function refreshTokens(config: OidcConfig, refreshToken: string): Promise<TokenSet> {
  const doc = await discover(config.issuer, config.internalBaseUrl);
  const body = new URLSearchParams({
    grant_type: 'refresh_token',
    refresh_token: refreshToken,
    client_id: config.clientId,
    client_secret: config.clientSecret,
  });
  const res = await fetch(internalEndpoint(config, doc.token_endpoint), {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body,
  });
  if (!res.ok) throw new Error(`Refresh fallo (${res.status}).`);
  return parseTokenResponse((await res.json()) as Record<string, unknown>);
}

/** Verifica un id_token contra el JWKS de Sytue ID y comprueba aud/nonce. */
export async function verifyIdToken(
  config: OidcConfig,
  idToken: string,
  expectedNonce?: string,
): Promise<JWTPayload> {
  const doc = await discover(config.issuer, config.internalBaseUrl);
  const jwksUrl = internalEndpoint(config, doc.jwks_uri);
  let jwks = jwksCache.get(jwksUrl);
  if (!jwks) {
    jwks = createRemoteJWKSet(new URL(jwksUrl));
    jwksCache.set(jwksUrl, jwks);
  }
  const { payload } = await jwtVerify(idToken, jwks, {
    issuer: config.issuer,
    audience: config.clientId,
    // EL ALGORITMO SE FIJA AQUÍ, no se acepta el que diga el token.
    //
    // Sytue ID firma con RS256 y solo con RS256 —`oidc/keys.go` no conoce otro, y
    // su discovery anuncia `id_token_signing_alg_values_supported: ["RS256"]`—,
    // así que aceptar cualquier otro no habilita nada legítimo y sí amplía la
    // superficie: la confusión de algoritmos es de donde salen casi todos los
    // fallos históricos de verificación de JWT. `jose` ya rechaza `none` y no
    // usaría una clave asimétrica para HMAC, o sea que hoy no hay nada
    // explotable; esto es que mañana tampoco lo haya si alguien publica una
    // clave nueva en el JWKS con otro `alg`.
    algorithms: ['RS256'],
  });
  if (expectedNonce && payload.nonce !== expectedNonce) {
    throw new Error('OIDC: nonce del id_token no coincide.');
  }
  return payload;
}

/**
 * Claims del id_token emitido por Sytue ID. Perfil OIDC estandar (RFC 7519 +
 * OpenID Connect Core) mas los claims propios de la suite: `roles` (scope
 * `roles`) y `groups` (tenancy `org/<slug>`). SIN `realm_access` (eso era
 * Keycloak); Sytue ID emite los roles directamente en el claim `roles`.
 */
interface SytueIdClaims extends JWTPayload {
  email?: string;
  email_verified?: boolean;
  name?: string;
  given_name?: string;
  family_name?: string;
  preferred_username?: string;
  picture?: string;
  locale?: string;
  roles?: string[];
  groups?: string[];
  /** Organización activa (slug). Sytue ID lo emite en el id_token y en el access
   *  token: ver services/id/internal/oidc/token.go. */
  tenant?: string;
  /** Hash del logotipo de la organización; solo en el id_token. */
  org_logo?: string;
  /** Color por defecto de la organización; solo en el id_token y solo si no es el de la casa. */
  org_color?: string;
  /** Forma por defecto de la organización; como `org_color`. */
  org_forma?: string;
  /** Clase de la organización activa, su plan y su cuota: ver services/id/internal/oidc/token.go. */
  org_kind?: string;
  plan?: string;
  storage_bytes?: number;
}

const KNOWN_ROLES: ReadonlySet<string> = new Set(['owner', 'admin', 'member', 'billing']);

/** Un valor de apariencia de la organización que va a una cookie: solo letras, o nada. */
const nombreDeApariencia = (v: string | undefined): string | undefined =>
  v !== undefined && /^[a-z]{1,16}$/.test(v) ? v : undefined;

/** Deriva el perfil Sytue desde los claims del id_token de Sytue ID. */
export function userFromClaims(claims: JWTPayload): SytueUser {
  const c = claims as SytueIdClaims;
  const roles = (c.roles ?? []).filter((r): r is SytueRole => KNOWN_ROLES.has(r));
  const groups = c.groups ?? [];
  // El claim `tenant` PRIMERO, que es el que Sytue ID emite de verdad
  // (oidc/token.go:99 y :195). La derivación por grupos se queda debajo como
  // respaldo, pero por sí sola no funcionaba: buscaba un grupo `org/<slug>` que
  // no emite nadie —`GroupsForUser` devuelve los NOMBRES de la tabla `groups`,
  // legibles, porque es lo que Drive matchea contra sus shares—, así que
  // `user.tenant` era `undefined` en todas las sesiones de producción.
  const orgGroup = groups.find((g) => g.startsWith('org/') || g.startsWith('/org/'));
  const tenant = c.tenant ?? orgGroup?.replace(/^\/?org\//, '');
  return {
    id: String(c.sub),
    email: c.email ?? '',
    emailVerified: Boolean(c.email_verified),
    name: c.name ?? c.preferred_username ?? c.email ?? 'Usuario',
    givenName: c.given_name,
    familyName: c.family_name,
    preferredUsername: c.preferred_username ?? c.email ?? String(c.sub),
    picture: c.picture,
    locale: c.locale ?? 'es',
    roles,
    groups,
    tenant,
    orgLogo: c.org_logo,
    // Son nombres (de color, de forma) y acaban en una cookie: solo letras, o nada.
    orgColor: nombreDeApariencia(c.org_color),
    orgForma: nombreDeApariencia(c.org_forma),
    orgKind: c.org_kind === 'personal' ? 'personal' : c.org_kind === 'org' ? 'org' : undefined,
    plan: c.plan,
    storageBytes: typeof c.storage_bytes === 'number' ? c.storage_bytes : undefined,
  };
}
