/**
 * Resolucion de la configuracion OIDC desde variables de entorno (nunca
 * literales en codigo). Cada superficie usa su client confidencial de Sytue ID
 * (servicio Go propio; docs/CONVENTIONS.md §8). Soberania 100% UE: el issuer
 * apunta a id.sytue.eu (prod) o id.sytue.localhost (dev), con discovery OIDC
 * estandar en /.well-known/openid-configuration. SIN rutas /realms/ (eso era
 * Keycloak, retirado del stack).
 */
import type { OidcConfig } from './types.js';

export interface EnvLike {
  [key: string]: string | undefined;
}

/** Server-to-server gateway calls use the cluster service before the public ingress. */
export function resolveIntegrationsUrl(env: EnvLike): string {
  return (
    env.SYTUE_INTEGRATIONS_INTERNAL_URL ??
    env.SYTUE_INTEGRATIONS_BASE_URL ??
    'https://integrations.sytue.eu'
  );
}

/**
 * Entorno EFECTIVO del servidor: fusiona el env de BUILD (`import.meta.env`, con
 * las PUBLIC_* inlineadas) con el de RUNTIME (`process.env`, las variables del
 * contenedor). En `astro dev` ambos coinciden; en el servidor SSR COMPILADO
 * (prod/built) `import.meta.env` quedó CONGELADO en el build, así que `process.env`
 * es la ÚNICA fuente viva de SYTUE_OIDC_ISSUER y demás — sin esto el BFF da 500 en
 * producción. El runtime GANA sobre el de build. Uso: `serverEnv(import.meta.env)`.
 */
export function serverEnv(buildEnv: EnvLike): EnvLike {
  const g = globalThis as { process?: { env?: EnvLike } };
  return { ...buildEnv, ...g.process?.env };
}

/**
 * Construye la configuracion OIDC de una superficie a partir del entorno.
 * Variables esperadas (por superficie):
 *   SYTUE_OIDC_ISSUER            https://id.sytue.eu (prod) / https://id.sytue.localhost (dev)
 *   SYTUE_OIDC_INTERNAL_BASE_URL  http://sytue-one-id (prod; opcional en local)
 *   SYTUE_OIDC_CLIENT_ID         p.ej. sytue-one
 *   SYTUE_OIDC_CLIENT_SECRET     (secreto; Scaleway Secret Manager / Sealed Secrets)
 *   SYTUE_OIDC_REDIRECT_URI      p.ej. https://one.sytue.eu/auth/callback
 *   SYTUE_OIDC_POST_LOGOUT_URI   p.ej. https://one.sytue.eu/
 *   SYTUE_OIDC_SCOPES            (opcional, separadas por espacio)
 */
export function resolveOidcConfig(env: EnvLike): OidcConfig {
  const issuer = normalizeBaseUrl(required(env, 'SYTUE_OIDC_ISSUER'), {
    key: 'SYTUE_OIDC_ISSUER',
    allowHttp: false,
  });
  const internalBaseUrl = env.SYTUE_OIDC_INTERNAL_BASE_URL
    ? normalizeBaseUrl(env.SYTUE_OIDC_INTERNAL_BASE_URL, {
        key: 'SYTUE_OIDC_INTERNAL_BASE_URL',
        allowHttp: true,
      })
    : undefined;
  const clientId = required(env, 'SYTUE_OIDC_CLIENT_ID');
  const clientSecret = required(env, 'SYTUE_OIDC_CLIENT_SECRET');
  const redirectUri = required(env, 'SYTUE_OIDC_REDIRECT_URI');
  const postLogoutRedirectUri =
    env.SYTUE_OIDC_POST_LOGOUT_URI ?? new URL('/', redirectUri).toString();
  if (env.SYTUE_COOKIE_DOMAIN?.trim()) {
    throw new Error(
      '@sytue/auth: SYTUE_COOKIE_DOMAIN no está permitido; las cookies deben ser host-only.',
    );
  }
  // `roles` NO va aquí: es un claim que Sytue ID emite siempre, no un scope de
  // scopes_supported. Pedirlo devolvía invalid_scope en authorize.
  const scopes = (env.SYTUE_OIDC_SCOPES ?? 'openid profile email offline_access')
    .split(/\s+/)
    .filter(Boolean);

  return {
    issuer,
    ...(internalBaseUrl ? { internalBaseUrl } : {}),
    clientId,
    clientSecret,
    redirectUri,
    postLogoutRedirectUri,
    scopes,
  };
}

function normalizeBaseUrl(value: string, options: { key: string; allowHttp: boolean }): string {
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`@sytue/auth: ${options.key} debe ser una URL absoluta válida.`);
  }
  const protocolAllowed =
    parsed.protocol === 'https:' || (options.allowHttp && parsed.protocol === 'http:');
  if (
    !protocolAllowed ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    (parsed.pathname !== '' && parsed.pathname !== '/')
  ) {
    throw new Error(
      `@sytue/auth: ${options.key} debe ser un origen sin credenciales, ruta, query ni fragmento.`,
    );
  }
  return parsed.origin;
}

function required(env: EnvLike, key: string): string {
  const value = env[key];
  if (!value) {
    throw new Error(`@sytue/auth: falta la variable de entorno ${key}.`);
  }
  return value;
}

/** Clave secreta para cifrar la cookie de sesion (HS256/A256GCM). */
export function resolveSessionSecret(env: EnvLike): string {
  const secret = env.SYTUE_SESSION_SECRET;
  if (!secret || secret.length < 32) {
    throw new Error('@sytue/auth: SYTUE_SESSION_SECRET debe definirse y tener >= 32 caracteres.');
  }
  return secret;
}
