/**
 * Guard de sesion del lado servidor. Protege paginas/endpoints Astro: exige
 * sesion valida y, opcionalmente, roles. Si no hay sesion, redirige a
 * /auth/login conservando returnTo. Roles owner/admin requieren MFA (la
 * politica MFA la impone Sytue ID; aqui solo autorizamos por rol).
 */
import { appDomains, returnToCanonico } from '@sytue/config';
import { translate } from '@sytue/i18n';
import { idiomaDeLaPeticion, paginaDeAviso } from './pagina-aviso.js';
import { sinAdminPorPlan } from '../types.js';
import type { SytuePermission, SytueRole, SytueSession } from '../types.js';
import { permsFromAccessToken } from '../perms.js';
import type { Bff, BffContext } from './bff.js';

export interface GuardResult {
  session: SytueSession;
  /** Permisos granulares efectivos del access token (vacío para owner/admin). */
  perms: SytuePermission[];
}

export interface GuardOptions {
  /** Roles permitidos; si se omite, basta con estar autenticado. */
  roles?: SytueRole[];
  /**
   * Permisos granulares que TAMBIÉN dan acceso (admin delegada). Basta con UNO.
   * Se combina en OR con `roles`: pasa quien tenga un rol permitido O un permiso.
   * Sin esta opción, un rol personalizado no entra aunque el backend le autorice.
   */
  perms?: SytuePermission[];
  /** Ruta de login del BFF (por defecto /auth/login). */
  loginPath?: string;
  /**
   * Clases de organización admitidas. `['org']` cierra la superficie a las
   * cuentas personales, que por rol entrarían: la persona es `owner` de su
   * propia organización personal. Un usuario sin el claim cuenta como `org`.
   */
  orgKinds?: ('org' | 'personal')[];
  /** Motivo específico del rechazo (por ejemplo 'solo_organizaciones'). */
  motivo?: string;
}

/**
 * Devuelve la sesion o una Response de redireccion/403 que el llamador debe
 * retornar. Patron de uso en una pagina Astro:
 *
 *   const guard = await requireSession(bff, ctx);
 *   if (guard instanceof Response) return guard;
 *   const { session } = guard;
 */
export async function requireSession(
  bff: Bff,
  ctx: BffContext,
  options: GuardOptions = {},
): Promise<GuardResult | Response> {
  const session = await bff.getSession(ctx);
  // El login vive BAJO el prefijo de la app (`/admin/auth/login`), no en la raíz
  // del host. Con `/auth/login` a secas, en producción esta redirección —la que
  // recibe cualquiera que abra la suite sin sesión— acababa en el 404 de
  // Traefik, y no se podía entrar en NINGUNA de las nueve apps.
  const loginPath = options.loginPath ?? `${bff.basePath}/auth/login`;
  if (!session) {
    // La petición puede llegar REESCRITA por dentro (entrada org-ful con la marca
    // `?__org=`, ADR 0056): el returnTo sale en la forma canónica, no en la
    // interna, o tras el login la barra se quedaría con la marca.
    const returnTo = returnToCanonico(ctx.url, bff.basePath);
    return new Response(null, {
      status: 302,
      headers: { location: `${loginPath}?returnTo=${encodeURIComponent(returnTo)}` },
    });
  }
  const perms = permsFromAccessToken(session.tokens.accessToken);
  if (options.orgKinds !== undefined && options.orgKinds.length > 0) {
    const kind = session.user.orgKind ?? 'org';
    if (!options.orgKinds.includes(kind)) return forbiddenPage(ctx, options.motivo);
  }
  const wantsRole = options.roles !== undefined && options.roles.length > 0;
  const wantsPerm = options.perms !== undefined && options.perms.length > 0;
  // SYTUE ADMIN SOLO CON SUSCRIPCIÓN. Pedir un rol o un permiso es pedir la
  // superficie de administración —en toda la suite solo la consola lo hace—, y
  // una cuenta personal sin plan de pago no la tiene aunque sea `owner` de su
  // organización personal. Va AQUÍ y no en cada página: la consola llama a esta
  // guarda desde diecinueve sitios entre páginas y endpoints, y una verja por
  // llamante es una verja que en el vigésimo se olvida.
  if ((wantsRole || wantsPerm) && sinAdminPorPlan(session.user)) {
    return forbiddenPage(ctx);
  }
  if (wantsRole || wantsPerm) {
    // OR entre ambos: un rol built-in permitido (owner/admin son superusuarios) o
    // cualquiera de los permisos delegados. Mismo criterio que HasPermission en Go.
    const byRole = wantsRole && options.roles!.some((r) => session.user.roles.includes(r));
    const byPerm = wantsPerm && options.perms!.some((p) => perms.includes(p));
    if (!byRole && !byPerm) {
      return forbiddenPage(ctx);
    }
  }
  return { session, perms };
}

/**
 * Página de «no tienes acceso» (403). Punto ÚNICO por el que pasan las páginas
 * protegidas de las diez apps, así que arreglarla aquí las arregla todas: antes
 * devolvía `No autorizado para este recurso.` en texto plano, en un solo idioma
 * y sin salida, y eso es lo que veía en producción cualquiera que abriera
 * admin.sytue.eu/sso sin ser admin.
 *
 * El documento lo pinta `paginaDeAviso`, compartido con el callback.
 */
function forbiddenPage(ctx: BffContext, motivo?: string): Response {
  const locale = idiomaDeLaPeticion(ctx);
  const esOrgOnly = motivo === 'solo_organizaciones';
  return paginaDeAviso({
    locale,
    titulo: translate(locale, esOrgOnly ? 'forbidden.org_only.title' : 'forbidden.title'),
    descripcion: translate(locale, esOrgOnly ? 'forbidden.org_only.body' : 'forbidden.description'),
    accion: translate(locale, esOrgOnly ? 'forbidden.org_only.action' : 'forbidden.action'),
    href: `${appDomains.one}/`,
    status: 403,
  });
}

/** ¿La sesión tiene alguno de estos permisos? owner/admin siempre pueden. */
export function hasAnyPerm(
  session: SytueSession,
  perms: SytuePermission[],
  granted?: SytuePermission[],
): boolean {
  if (session.user.roles.includes('owner') || session.user.roles.includes('admin')) return true;
  const have = granted ?? permsFromAccessToken(session.tokens.accessToken);
  return perms.some((p) => have.includes(p));
}

/** Comprueba si el usuario tiene alguno de los roles indicados. */
export function hasAnyRole(session: SytueSession, roles: SytueRole[]): boolean {
  return roles.some((r) => session.user.roles.includes(r));
}

/**
 * Si la petición es una navegación que viene de OTRO sitio (FE-02).
 *
 * Un GET que cambia algo —crear un documento desde un fichero de Drive, dar de alta
 * una foto— no tiene la protección de `checkOrigin` de Astro, que solo mira los
 * métodos que escriben, y la cookie de sesión es `SameSite=Lax`: viaja en una
 * navegación de primer nivel desde cualquier web. `Sec-Fetch-Site` lo pone el
 * navegador y el JS de una página no puede falsificarlo. «none» es abrir el enlace
 * desde fuera del navegador (las apps nativas), y vale; un navegador sin la cabecera,
 * también: no hay con qué distinguirlo.
 */
export function vieneDeOtroSitio(request: Request): boolean {
  return request.headers.get('sec-fetch-site') === 'cross-site';
}
