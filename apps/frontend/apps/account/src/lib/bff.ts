/**
 * BFF de Mi cuenta (solo servidor). Client OIDC confidencial `sytue-account`.
 * El secreto del client se resuelve del entorno; nunca llega al navegador.
 * docs/CONVENTIONS.md §8.
 *
 * El montaje —config OIDC, secreto, `basePath`, cookies— y los dos guards están
 * en `crearBffDeApp` (`@sytue/auth/server`): eran el MISMO fichero en siete apps.
 * Lo de abajo sí es de esta app, y por eso se queda.
 */
import type { APIContext } from 'astro';
import { crearBffDeApp, serverEnv, toBffContext } from '@sytue/auth/server';
import { cabecerasConToken } from './cabeceras.js';

export const { bff, requirePageSession, getApiSession } = crearBffDeApp(import.meta.env);
export { toBffContext };

const env = serverEnv(import.meta.env);

/**
 * Sytue ID por el Service interno: desde un pod el nombre público no resuelve.
 * El issuer es solo el respaldo (mismo criterio que `apps/portal/src/lib/auth.ts`).
 */
function idBaseUrl(): string {
  return (env.SYTUE_ID_INTERNAL_URL ?? env.SYTUE_OIDC_ISSUER ?? 'https://id.sytue.eu').replace(
    /\/$/,
    '',
  );
}

/**
 * Reenvía a Sytue ID con el access token de la sesión. Devuelve 401 sin sesión.
 * El token NUNCA sale al navegador: se añade aquí, en el servidor.
 */
export async function proxyId(ctx: APIContext, path: string, init?: RequestInit) {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session?.tokens.accessToken) return new Response(null, { status: 401 });
  return fetch(`${idBaseUrl()}${path}`, {
    ...init,
    headers: cabecerasConToken(init?.headers, session.tokens.accessToken),
  });
}
