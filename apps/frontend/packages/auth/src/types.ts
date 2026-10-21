/** Tipos compartidos de @sytue/auth (OIDC contra Sytue ID, servicio Go propio). */
import { esSoloDeOrganizaciones } from '@sytue/config';

/** Roles de la suite (ver docs/CONVENTIONS.md §8). */
export type SytueRole = 'owner' | 'admin' | 'member' | 'billing';

/**
 * Permisos GRANULARES de administración (admin delegada). Espejo EXACTO del
 * catálogo fijo del backend — `services/id/internal/domain/roles.go`: si allí se
 * añade uno, hay que añadirlo aquí (el guard es deny-by-default, así que olvidarlo
 * solo deja la capacidad inaccesible desde la UI, nunca la abre de más).
 *
 * `owner` y `admin` son superusuarios y NO dependen de estos permisos (mismo
 * criterio que `HasPermission` en Go): conservan acceso total.
 */
export type SytuePermission =
  | 'users.manage'
  | 'users.read'
  | 'sso.manage'
  | 'apps.manage'
  | 'access_policies.manage'
  | 'groups.manage'
  | 'audit.view'
  | 'billing.manage'
  | 'roles.manage';

/** Perfil de usuario derivado del id_token / userinfo de Sytue ID. */
export interface SytueUser {
  /** `sub` del token. */
  id: string;
  email: string;
  emailVerified: boolean;
  name: string;
  givenName?: string | undefined;
  familyName?: string | undefined;
  preferredUsername: string;
  picture?: string | undefined;
  /** Locale (es_ES por defecto en el tema sytue). */
  locale?: string;
  /** Roles emitidos por Sytue ID (claim `roles`). */
  roles: SytueRole[];
  /** Grupos de tenancy de Sytue ID (`org/<slug>`). */
  groups: string[];
  /** Organización activa (claim `tenant`). */
  tenant?: string | undefined;
  /**
   * Hash del logotipo propio de la organización (claim `org_logo`), ausente si no
   * ha subido ninguno. No es el logo: es lo que permite componer su URL con
   * `?v=<hash>` y pedirla como inmutable sin dejar de verse al cambiarla.
   */
  orgLogo?: string | undefined;
  /**
   * Color por defecto de la organización (claim `org_color`), ausente si es el de
   * la casa. Lo ve quien no ha elegido el suyo en su menú de cuenta.
   */
  orgColor?: string | undefined;
  /** Forma por defecto de la organización (claim `org_forma`), ausente si es la de la casa. */
  orgForma?: string | undefined;
  /**
   * Clase de la organización activa (claim `org_kind`): `org` o `personal`.
   * Una cuenta personal —una persona cuya organización es su correo— no tiene
   * Sytue Admin ni SyERP, y es lo que decide si el portal pinta esas teselas.
   * Ausente en tokens anteriores a la distinción, que se leen como `org`.
   */
  orgKind?: 'org' | 'personal' | undefined;
  /** Contrato vigente (claim `plan`): `personal_free`, `pro` (200 GB/usuario) o `enterprise` (1 TB/usuario, Soporte Premium). */
  plan?: string | undefined;
  /** Cuota de almacenamiento que concede el plan, en bytes (claim `storage_bytes`); solo en cuentas personales. */
  storageBytes?: number | undefined;
}

/** Si la sesión es de una cuenta personal o individual. Sin el claim, no lo es. */
export function esCuentaPersonal(
  user: Pick<SytueUser, 'orgKind'> & Partial<Pick<SytueUser, 'plan' | 'tenant'>>,
): boolean {
  if (user.orgKind === 'personal') return true;
  if (
    user.plan === 'personal_free' ||
    (user.plan !== undefined && user.plan.startsWith('personal_'))
  )
    return true;
  if (user.tenant?.startsWith('p-') || user.tenant?.startsWith('personal-')) return true;
  return false;
}

/**
 * Planes que cuestan dinero. Espejo de `commerce.PlanPro` y `PlanEnterprise`
 * (platform/pkg/commerce): `personal_free` no está, y un claim ausente o
 * desconocido tampoco cuenta como pagado. Lo que falta nunca abre puertas.
 *
 * Los nombres son los de la migración 0064 (`standard`→`pro`, `pro`→`enterprise`,
 * ADR 0038). Este conjunto se quedó con los VIEJOS cuando se renombró todo lo
 * demás, así que `enterprise` —el plan de 25 €— no contaba como pagado y
 * `sinAdminPorPlan` le cerraba Sytue Admin a una cuenta individual que sí paga.
 */
const PLANES_DE_PAGO: ReadonlySet<string> = new Set(['pro', 'enterprise']);

/** Si la sesión lleva un plan de pago vigente (claim `plan`). */
export function esPlanDePago(user: Pick<SytueUser, 'plan'>): boolean {
  return user.plan !== undefined && PLANES_DE_PAGO.has(user.plan);
}

/**
 * Si a esta sesión se le cierra Sytue Admin por su plan: cuenta personal SIN
 * plan de pago. Es la regla del producto —«Sytue Admin solo con suscripción»,
 * que es lo que promete el sitio público— escrita una sola vez: la leen la
 * guarda del BFF, las secciones de la consola y la tesela del conmutador.
 * Una organización nunca se cierra por aquí: su plan no acota Admin.
 */
export function sinAdminPorPlan(user: Pick<SytueUser, 'orgKind' | 'plan'>): boolean {
  return esCuentaPersonal(user) && !esPlanDePago(user);
}

/**
 * ¿Le está CERRADO este producto a esta sesión por ser solo de organizaciones?
 *
 * Sytue ERP y Sytue Web (`PRODUCTOS_SOLO_ORGANIZACION` de @sytue/config) son de
 * organizaciones: una cuenta personal —donde la organización es la propia persona— no
 * los abre (migración 0063 de ID, `RequireOrgKind("org")` en Go y `requireSession` con
 * `orgKinds: ['org']` en Astro). Sin sesión no se cierra nada: quien no ha entrado no es
 * una cuenta personal, y el login decide después.
 */
export function soloOrganizacion(
  key: string,
  user?: (Pick<SytueUser, 'orgKind'> & Partial<Pick<SytueUser, 'plan' | 'tenant'>>) | null,
): boolean {
  return esSoloDeOrganizaciones(key) && !!user && esCuentaPersonal(user);
}

/** Conjunto de tokens OIDC. */
export interface TokenSet {
  accessToken: string;
  refreshToken?: string | undefined;
  idToken?: string | undefined;
  /** Epoch ms en que expira el access token. */
  expiresAt: number;
  scope?: string | undefined;
  tokenType: string;
}

/** Sesion del lado servidor (almacenada cifrada en la cookie de sesion). */
export interface SytueSession {
  user: SytueUser;
  tokens: TokenSet;
  /** Client OIDC con el que se inicio (p.ej. `sytue-one`). */
  clientId: string;
}

/** Configuracion OIDC de una superficie (BFF confidencial). */
export interface OidcConfig {
  /** Issuer de Sytue ID: https://id.sytue.eu (prod) / https://id.sytue.localhost (dev). */
  issuer: string;
  /**
   * Origen privado para las llamadas servidor-a-servidor del BFF. El issuer y
   * las URL que recibe el navegador siguen siendo públicos; únicamente
   * discovery, token y JWKS se resuelven por la red interna del clúster.
   */
  internalBaseUrl?: string;
  clientId: string;
  /** Secreto del client confidencial (solo servidor; nunca al cliente). */
  clientSecret: string;
  /** URL de callback del BFF, p.ej. https://one.sytue.eu/auth/callback. */
  redirectUri: string;
  /** URL post-logout. */
  postLogoutRedirectUri: string;
  scopes?: string[];
}

export interface DiscoveryDocument {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint: string;
  end_session_endpoint: string;
  jwks_uri: string;
}
