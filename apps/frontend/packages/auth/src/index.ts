/**
 * @sytue/auth — autenticacion OIDC (Authorization Code + PKCE) contra Sytue ID.
 *
 * - `@sytue/auth/server`: flujo OIDC, BFF (endpoints Astro), cookie de sesion
 *   httpOnly de la suite (.sytue.eu), guard de sesion/roles. SOLO servidor.
 * - `@sytue/auth/client`: utilidades de navegador (sin tokens).
 *
 * Este punto de entrada raiz expone solo los tipos compartidos para evitar
 * cargar codigo de servidor en el cliente por accidente.
 */
export type {
  SytueUser,
  SytueRole,
  SytueSession,
  TokenSet,
  OidcConfig,
  DiscoveryDocument,
  SytuePermission,
} from './types.js';
export { esCuentaPersonal, esPlanDePago, sinAdminPorPlan, soloOrganizacion } from './types.js';
