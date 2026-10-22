/** @sytue/auth/server — flujo OIDC + BFF + guard (solo lado servidor). */
export { createBff } from './bff.js';
export { crearBffDeApp, sesionApi, toBffContext } from './app-bff.js';
export type { BffDeApp, ServerSession } from './app-bff.js';
export { rutaDeRetornoDeApp } from './consentimiento.js';
export { rutasDeServiciosVinculados } from './linked-services.js';
export type { Bff, BffContext, BffOptions, CookieJar } from './bff.js';
export { rutasDeAuth } from './rutas.js';
export { requireSession, hasAnyRole, hasAnyPerm, vieneDeOtroSitio } from './guard.js';
export type { GuardOptions, GuardResult } from './guard.js';
export { permsFromAccessToken, ALL_PERMISSIONS } from '../perms.js';
export {
  SESSION_COOKIE,
  OIDC_STATE_COOKIE,
  seal,
  unseal,
  sealSession,
  readSession,
  sessionCookieAttributes,
  hostCookieName,
  DEVICE_COOKIE,
  deviceCookieHeader,
} from './cookies.js';
export {
  discover,
  buildAuthorizationRequest,
  exchangeCode,
  refreshTokens,
  verifyIdToken,
  userFromClaims,
} from '../oidc.js';
export {
  resolveOidcConfig,
  resolveSessionSecret,
  resolveIntegrationsUrl,
  serverEnv,
} from '../config.js';
export { consulta } from './consulta.js';
export { crearDirectorio } from './directorio.js';
export { rutasDePeticiones } from './peticiones.js';
export { rutasDeCompartir } from './compartir.js';
export type { OpcionesCompartir, PersonaCompartida } from './compartir.js';
export type { OpcionesPeticiones } from './peticiones.js';
export type { Directorio, Persona } from './directorio.js';
export type { EnvLike } from '../config.js';
export { crearProxyDeServicio, rutaQueEscapa } from './proxy.js';
export type { OpcionesProxyDeServicio } from './proxy.js';
