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
  resolveDriveServiceUrl,
  resolveRoomsServiceUrl,
  serverEnv,
} from '../config.js';
export {
  crearRutaBlobsCifrados,
  crearRutaMatrixStorageBootstrap,
  type OpcionesRutaBlobsCifrados,
  type OpcionesRutaMatrixStorageBootstrap,
} from './almacenamiento-cifrado.js';
export { consulta } from './consulta.js';
export { crearDirectorio, respuestaAgenda } from './directorio.js';
export { rutasDePeticiones } from './peticiones.js';
export { rutasDeCompartir } from './compartir.js';
export type { OpcionesCompartir, PersonaCompartida } from './compartir.js';
export type { OpcionesPeticiones } from './peticiones.js';
export type { Directorio, Persona } from './directorio.js';
export type { EnvLike } from '../config.js';
export { crearProxyDeServicio, rutaQueEscapa } from './proxy.js';
export type { OpcionesProxyDeServicio } from './proxy.js';
export { rutaBootstrapMatrixPrivado } from './matrix-storage.js';
export { rutasRecuperacionMatrix } from './matrix-recovery.js';
export { rutaBlobsMatrixPrivados } from './matrix-storage-blobs.js';
export { rutaMigracionMatrixPrivada } from './matrix-storage-migration.js';

export {
  rutaAutoridadMatrixPrivada,
  rutaPublicacionesMatrixPrivadas,
} from './matrix-storage-authority.js';

// Public maintained client core; shares the transport without requiring a data session.
export { clientCoreHead, clientCoreProxy, roomsServiceBase } from '@sytue/api-clients';

export { rutaMailPrivado, rutaCorteIdentidadMatrixPrivada } from './mail-private-storage.js';

export { rutaMetadataMailPrivado } from './mail-private-metadata.js';
export { rutaBovedaPq } from './pq-vault.js';
