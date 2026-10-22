/**
 * Guard de sesion del lado servidor. Protege paginas/endpoints Astro: exige
 * sesion valida y, opcionalmente, roles. Si no hay sesion, redirige a
 * /auth/login conservando returnTo. Roles owner/admin requieren MFA (la
 * politica MFA la impone Sytue ID; aqui solo autorizamos por rol).
 */
import { appDomains, brand, fontFamily, returnToCanonico, themeColors } from '@sytue/config';
import { LOCALE_COOKIE, htmlLangAttrs, isLocale, negotiateLocale, translate } from '@sytue/i18n';
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
 * ponytail: documento suelto con los tokens en línea, NO componentes de
 * @sytue/ui. El guard es una librería de servidor sin Preact ni el CSS de la app
 * (cada app lo compila con su propio hash), así que no hay bundle que reutilizar;
 * traerse preact-render-to-string y @sytue/ui a `@sytue/auth` por una pantalla de
 * error costaría más que las quince líneas de estilo que hay aquí. Si algún día
 * la suite sirve una hoja de estilo compartida en una URL estable, esto se
 * sustituye por un `<link>` y el marcado se queda igual.
 */
function forbiddenPage(ctx: BffContext, motivo?: string): Response {
  // La preferencia explícita se lee por el JAR de cookies —el mismo por el que el
  // BFF lee la sesión— y no por la cabecera cruda: `Cookie` es un nombre de
  // cabecera prohibido, así que hay entornos que la descartan en silencio y el
  // idioma elegido por el usuario se perdería sin dar la cara. El resto de la
  // negociación (Accept-Language > inglés) lo sigue decidiendo `negotiateLocale`.
  const elegida = ctx.cookies.get(LOCALE_COOKIE)?.value;
  const locale = isLocale(elegida)
    ? elegida
    : negotiateLocale(
        ctx.request.headers.get('accept-language'),
        ctx.request.headers.get('cookie'),
      );
  const { lang, dir } = htmlLangAttrs(locale);
  const esOrgOnly = motivo === 'solo_organizaciones';
  const titulo = esOrgOnly
    ? translate(locale, 'forbidden.org_only.title')
    : translate(locale, 'forbidden.title');
  const descripcion = esOrgOnly
    ? translate(locale, 'forbidden.org_only.body')
    : translate(locale, 'forbidden.description');
  const accion = esOrgOnly
    ? translate(locale, 'forbidden.org_only.action')
    : translate(locale, 'forbidden.action');

  const html = `<!doctype html>
<html lang="${lang}" dir="${dir}" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>${titulo} — Sytue One</title>
<style>
:root{color-scheme:light;--bg:${themeColors.light.bg};--fg:${themeColors.light.text};--muted:${themeColors.light['text-muted']};--link:${brand.primary[700]};--ring:${brand.primary[500]}}
*{box-sizing:border-box}
body{margin:0;min-height:100dvh;display:grid;place-items:center;padding:2rem;
background:var(--bg);color:var(--fg);font-family:${fontFamily.sans};
-webkit-font-smoothing:antialiased}
main{max-width:34rem;text-align:center}
h1{margin:0;font-size:1.5rem;line-height:1.3;font-weight:600;letter-spacing:-0.01em}
p{margin:0.75rem 0 0;font-size:1rem;line-height:1.6;color:var(--muted)}
a{display:inline-block;margin-top:1.75rem;color:var(--link);font-size:1rem;font-weight:600;
text-decoration:underline;text-underline-offset:3px;border-radius:0.25rem}
a:focus-visible{outline:2px solid var(--ring);outline-offset:3px}
</style>
</head>
<body>
<main>
<h1>${titulo}</h1>
<p>${descripcion}</p>
<a href="${appDomains.one}/">${accion}</a>
</main>
</body>
</html>`;

  return new Response(html, {
    status: 403,
    headers: { 'content-type': 'text/html; charset=utf-8', 'cache-control': 'no-store' },
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
