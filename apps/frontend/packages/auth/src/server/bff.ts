/**
 * BFF (Backend-for-Frontend) de autenticacion para apps Astro.
 *
 * Crea los handlers de los endpoints de servidor que implementan el flujo
 * OIDC Authorization Code + PKCE y emiten la cookie de sesion httpOnly de la
 * suite (.sytue.eu). Disenado para Astro (`APIContext`-compatible) pero sin
 * acoplarse a Astro: trabaja sobre Request/Response estandar y un adaptador de
 * cookies minimo. El secreto del client NUNCA llega al navegador. Cada BFF usa
 * una cookie host-only; el SSO entre subdominios lo aporta el IdP mediante OAuth.
 *
 * Endpoints tipicos por app (en src/pages/auth/):
 *   GET  /auth/login      -> redirige a Sytue ID (guarda state/nonce/verifier)
 *   GET  /auth/callback   -> intercambia code, verifica id_token, emite sesion
 *   POST /auth/logout     -> borra sesion local y redirige a end_session (SSO)
 *   GET  /auth/session    -> JSON del usuario (o 401) para hidratar la shell
 *   POST /auth/refresh    -> refresca tokens si procede
 *   GET  /auth/avatar     -> la foto propia de Sytue ID, pedida con el token
 */
import { ORG_COLOR_COOKIE, ORG_FORMA_COOKIE } from '@sytue/config';
import type { OidcConfig, SytueSession, SytueUser, TokenSet } from '../types.js';
import {
  buildAuthorizationRequest,
  discover,
  exchangeCode,
  refreshTokens,
  userFromClaims,
  verifyIdToken,
} from '../oidc.js';
import {
  OIDC_STATE_COOKIE,
  SESSION_COOKIE,
  hostCookieName,
  readSession,
  seal,
  sealSession,
  sessionCookieAttributes,
  unseal,
} from './cookies.js';

/** Adaptador minimo de cookies (compatible con Astro.cookies). */
export interface CookieJar {
  get: (name: string) => { value: string } | undefined;
  set: (name: string, value: string, options?: Record<string, unknown>) => void;
  delete: (name: string, options?: Record<string, unknown>) => void;
}

export interface BffContext {
  request: Request;
  cookies: CookieJar;
  url: URL;
}

export interface BffOptions {
  config: OidcConfig;
  sessionSecret: string;
  /** Duracion de la cookie de sesion en segundos (por defecto 30 dias). */
  sessionMaxAge?: number;
  /** Secure=false solo en dev local sobre http (por defecto secure=true). */
  secureCookies?: boolean;
  /**
   * Prefijo bajo el que se sirve esta app (`base` de Astro: `/admin`, `/one`,
   * `/drive`…). Se pasa `import.meta.env.BASE_URL` desde la app.
   *
   * SIN esto, todo lo que el servidor construye a mano —la redirección al login
   * del guard, el returnTo por defecto— apunta a la raíz del host, donde no hay
   * nada: en producción cada app vive BAJO su prefijo y la raíz devuelve el 404
   * de Traefik. Era literalmente imposible iniciar sesión en ninguna:
   * `admin.sytue.eu/` → `/admin` → `/auth/login?returnTo=/admin` → 404.
   */
  basePath?: string;
}

/** `/admin/` → `/admin`; `/` o vacío → ``. Deja el prefijo listo para concatenar. */
function normalizeBase(base: string | undefined): string {
  const b = (base ?? '/').trim();
  if (b === '' || b === '/') return '';
  const prefixed = b.startsWith('/') ? b : `/${b}`;
  let end = prefixed.length;
  while (end > 0 && prefixed.charCodeAt(end - 1) === 47) end--;
  return prefixed.slice(0, end);
}

interface OidcState {
  state: string;
  nonce: string;
  codeVerifier: string;
  /** Ruta a la que volver tras el login. */
  returnTo: string;
}

/**
 * Lo que viaja en el parámetro `state` del OAuth, sellado.
 *
 * EL DESTINO VA AQUÍ Y NO SOLO EN LA COOKIE porque el flujo puede empezar en un
 * host y terminar en otro. Medido en producción: `docs.sytue.eu/docs/edit?id=X`
 * manda al IdP con `redirect_uri` en `<organizacion>.sytue.eu`, así que el
 * callback se ejecuta donde la cookie de estado —host-only— no existe, el flujo
 * se reinicia y el reinicio no sabía a dónde iba: se acababa en la lista de
 * documentos, con el `?id=` perdido. En el `state` el destino sobrevive al salto.
 *
 * NO debilita nada: el `state` va cifrado con el secreto de sesión (nadie lo
 * escribe desde fuera), el `returnTo` pasa igualmente por `safeReturnTo`, y el
 * code NUNCA se canjea sin el `codeVerifier` de la cookie, que es la defensa
 * real. Sin cookie solo se reinicia el login, ahora conservando el destino.
 */
interface StateExterno {
  /** El state aleatorio de siempre; se compara con el de la cookie. */
  s: string;
  /** Destino, tal cual llegó a `/auth/login`. */
  r: string;
}

const DEFAULT_MAX_AGE = 60 * 60 * 24 * 30;

export function createBff(opts: BffOptions) {
  const { config, sessionSecret } = opts;
  const basePath = normalizeBase(opts.basePath);
  const maxAge = opts.sessionMaxAge ?? DEFAULT_MAX_AGE;
  const secure = opts.secureCookies ?? true;
  const cookieAttrs = sessionCookieAttributes({ maxAge, secure });
  const sessionCookie = hostCookieName(SESSION_COOKIE, secure);
  const stateCookie = hostCookieName(OIDC_STATE_COOKIE, secure);
  // Atributos del BORRADO. `secure` NO es decorativo aqui: el navegador rechaza
  // ENTERO cualquier Set-Cookie de un nombre `__Host-` que no lo lleve —tambien
  // el de borrado—, asi que sin esto la cookie de sesion sobrevivia al logout en
  // las diez superficies y volver a la app seguia entrando. Path=/ por lo mismo.
  const clearAttrs = { path: '/', secure };

  // La sesión y, a su lado, el color de su organización para el script del primer
  // pintado (ADR 0071). Esa segunda cookie NO es identidad —lleva el nombre de un
  // color— y por eso la puede leer el navegador; es host-only como la de sesión,
  // porque cada app puede tener activa una organización distinta. Sin color propio
  // se borra, y la hoja pinta el de la casa.
  const escribirSesion = async (ctx: BffContext, session: SytueSession): Promise<void> => {
    ctx.cookies.set(sessionCookie, await sealSession(session, sessionSecret), cookieAttrs);
    // La apariencia de la organización, al lado y legible: un eje por cookie.
    for (const [cookie, valor] of [
      [ORG_COLOR_COOKIE, session.user.orgColor],
      [ORG_FORMA_COOKIE, session.user.orgForma],
    ] as const) {
      if (valor) ctx.cookies.set(cookie, valor, { ...cookieAttrs, httpOnly: false });
      else ctx.cookies.delete(cookie, clearAttrs);
    }
  };

  // Borra la cookie de dominio emitida por versiones anteriores. Las nuevas
  // usan __Host- y nunca se comparten con publicaciones de usuarios.
  const clearLegacyDomainCookie = (ctx: BffContext): void => {
    const labels = new URL(config.redirectUri).hostname.split('.');
    if (labels.length < 2) return;
    ctx.cookies.delete(SESSION_COOKIE, { path: '/', domain: `.${labels.slice(-2).join('.')}` });
  };

  /**
   * Sesión renovada a partir de los tokens NUEVOS, perfil incluido.
   *
   * Los dos sitios que refrescan hacían `{ ...session, tokens }`, que conserva el
   * `user` de cuando se inició la sesión: nada de lo que cambiara en el IdP —el
   * logo de la organización, el rol, el nombre— llegaba hasta cerrar sesión y
   * volver a entrar. Y el que más se ejecuta no es el handler `refresh`, es el
   * refresco transparente de `getSession`.
   *
   * Si el id_token nuevo no se puede leer, se conserva el perfil anterior:
   * quedarse con un dato viejo es mejor que tirar abajo una sesión válida.
   */
  const nextSession = async (session: SytueSession, tokens: TokenSet): Promise<SytueSession> => {
    if (!tokens.idToken) return { ...session, tokens };
    try {
      const claims = await verifyIdToken(config, tokens.idToken);
      return { ...session, tokens, user: userFromClaims(claims) };
    } catch {
      return { ...session, tokens };
    }
  };

  /**
   * El `picture` del id_token, reescrito a la ruta `/auth/avatar` de ESTA app.
   *
   * Sytue ID lo emite como `https://id.<dominio>/account/avatar?v=<versión>`, una
   * ruta que solo responde con SU cookie de sesión. Medido en producción el
   * 2026-09-24: esa URL da 404 —en `id.<dominio>` el `/account` lo sirve la app
   * «Mi cuenta», que no tiene esa página— y, aunque llegara al IdP, su cookie dura
   * 12 h y la de la app 30 días. El `<img>` de la barra salía roto en las trece
   * superficies. Del claim solo se aprovecha la versión: la foto la trae el BFF
   * con el token que ya tiene, desde el mismo origen que la página.
   *
   * La versión va en la URL para que cambiar la foto cambie la URL: el claim se
   * relee en cada refresco del token (`nextSession`), así que ninguna app se
   * queda con la foto vieja en caché más allá de lo que dura un access token.
   */
  const conAvatarPropio = (session: SytueSession): SytueSession => {
    const pic = session.user.picture;
    const propia = `${basePath}/auth/avatar`;
    if (!pic || pic.startsWith(propia)) return session;
    let v = '';
    try {
      v = new URL(pic).searchParams.get('v') ?? '';
    } catch {
      // Un claim que no es URL absoluta no trae versión; la foto se sirve igual.
    }
    const user: SytueUser = {
      ...session.user,
      picture: v ? `${propia}?v=${encodeURIComponent(v)}` : propia,
    };
    return { ...session, user };
  };

  return {
    /**
     * Prefijo bajo el que se sirve esta app (`/admin`, `/one`… o `` en la raíz).
     * Lo lee el guard para redirigir al login que EXISTE: en producción cada app
     * vive bajo su prefijo y `/auth/login` a secas es un 404 de Traefik.
     */
    basePath,

    /** GET /auth/login */
    async login(ctx: BffContext): Promise<Response> {
      // EL FLUJO EMPIEZA EN EL HOST DONDE TERMINA. La cookie de estado es host-only
      // y el IdP devuelve siempre al host de `redirect_uri`; si el login arranca en
      // un host DISTINTO del del callback, el navegador llega sin cookie, el login
      // se reinicia y cuesta OTRA vuelta entera por el IdP. Pasó con el callback en
      // el host del tenant (medido en producción: siete redirecciones y dos
      // `/oauth2/authorize` por entrada —el doble de carga para el limitador por
      // ráfaga del IdP, y lo que dejaba al barrido de pantallas parado en
      // `/oauth2/authorize`—) y volvería a pasar entrando por el host de
      // organización una vez que la canónica es la de producto (ADR 0055). Saltando
      // antes al host del callback, la cookie nace donde se va a leer: una sola
      // vuelta.
      //
      // Solo con `https:`. Si el host de la petición no es de confianza, Astro no
      // cree `X-Forwarded-Host` y `ctx.url` cae a `http://localhost:<puerto>`
      // (ver `astro-security.ts`); redirigir ahí sería un bucle, así que en ese
      // caso se sigue como siempre. El destino sale de la configuración, no de la
      // petición: no abre ninguna redirección.
      const callback = new URL(config.redirectUri);
      if (ctx.url.protocol === 'https:' && ctx.url.host !== callback.host) {
        // VETO al pathname protocol-relative: `//host/...` en la petición se
        // resolvería contra OTRO host. Y no basta con anteponer una barra:
        // `///host/...` resuelve IGUAL de cruzado —el parser salta TODAS las
        // barras iniciales de un esquema especial—, así que se colapsan a una
        // sola y la ruta rara la atiende (o 404ea) la propia app, nunca fuera.
        const pathname = ctx.url.pathname.replace(/^\/+/, '/');
        const destino = new URL(pathname + ctx.url.search, callback.origin);
        return Response.redirect(destino.toString(), 302);
      }
      clearLegacyDomainCookie(ctx);
      // CON el prefijo de la app, lo traiga o no. El destino se resuelve en el host
      // canónico —ahí corre el callback (desde el ADR 0055, el de PRODUCTO)—, donde
      // la raíz es el Hub. Pero en el host propio la app también atiende `/` (Astro
      // enruta aunque falte el `base`), y quien construye el `returnTo` con
      // `url.pathname` —las ~60 páginas de la suite y el guard— manda `/?id=X`.
      // Medido el 2026-09-25: `rooms.sytue.eu/?id=X` → `returnTo=/?id=X` →
      // `corporate.sytue.eu/?id=X`, el Hub en vez de la reunión. Aquí pasan TODOS
      // los logins, así que se arregla una vez para las trece apps.
      const returnTo = conPrefijo(basePath, ctx.url.searchParams.get('returnTo') ?? '/');
      const authReq = await buildAuthorizationRequest(config);
      const state: OidcState = {
        state: authReq.state,
        nonce: authReq.nonce,
        codeVerifier: authReq.pkce.codeVerifier,
        returnTo,
      };
      const sealed = await seal(state, sessionSecret);
      // El mismo `state` aleatorio que guarda la cookie, más el destino, sellados
      // en el parámetro que el IdP devuelve tal cual (lo trata como opaco).
      const externo: StateExterno = { s: authReq.state, r: returnTo };
      const url = new URL(authReq.url);
      url.searchParams.set('state', await seal(externo, sessionSecret));
      // `reauth=1`: una acción sensible ha pedido confirmar la identidad (step-up,
      // `insufficient_user_authentication`). `max_age=0` (OIDC Core §3.1.2.1) obliga a
      // Sytue ID a autenticar de nuevo aunque la sesión SSO siga viva, y el token que
      // vuelve lleva un `auth_time` fresco.
      if (ctx.url.searchParams.get('reauth') === '1') url.searchParams.set('max_age', '0');
      // Cookie de estado efimera (30 min: la gente tarda en el formulario del IdP),
      // host-only, httpOnly. Si aun asi caduca, el callback reinicia el flujo.
      ctx.cookies.set(stateCookie, sealed, {
        path: '/',
        httpOnly: true,
        secure,
        sameSite: 'lax',
        maxAge: 1800,
      });
      return Response.redirect(url.toString(), 302);
    },

    /** GET /auth/callback */
    async callback(ctx: BffContext): Promise<Response> {
      const code = ctx.url.searchParams.get('code');
      const returnedState = ctx.url.searchParams.get('state');
      const error = ctx.url.searchParams.get('error');
      if (error) {
        // TERMINAL, nunca una redirección. Rebotar a `/?auth_error=...` metía al
        // navegador en un bucle infinito: el guard de `/` no lee ese parámetro y
        // reinicia el login, que vuelve a fallar por lo mismo. El usuario veía
        // ERR_TOO_MANY_REDIRECTS y el motivo real (p. ej. invalid_scope por una
        // configuración incorrecta del client) quedaba oculto.
        ctx.cookies.delete(stateCookie, clearAttrs);
        return authErrorResponse(error, ctx.url.searchParams.get('error_description'));
      }
      if (!code || !returnedState) return redirect(`${basePath}/auth/login`);

      // El `state` de vuelta trae dentro el aleatorio y el destino. Un `state`
      // plano es un flujo empezado antes de este cambio (o por otro cliente): se
      // acepta como el aleatorio a secas, para no tirar los logins en vuelo
      // durante un despliegue.
      const externo = await unseal<StateExterno>(returnedState, sessionSecret);
      const stateInterno = externo?.s ?? returnedState;

      const stored = await unseal<OidcState>(ctx.cookies.get(stateCookie)?.value, sessionSecret);
      if (!stored || stored.state !== stateInterno) {
        // Estado ausente/caducado/desincronizado (p. ej. el usuario tardó más que
        // la vida de la cookie en el formulario del IdP, o pestañas en paralelo).
        // NO es un fallo terminal: se descarta el code (nunca se acepta sin su
        // verifier, que es la defensa CSRF real) y se REINICIA el flujo con estado
        // fresco. Con la sesión SSO del IdP viva, el reintento es transparente.
        ctx.cookies.delete(stateCookie, clearAttrs);
        // CON el destino: este reinicio es el caso normal cuando el callback cae
        // en un host distinto del que empezó, y sin esto aterrizaba siempre en la
        // portada de la app.
        const destino = externo ? safeReturnTo(externo.r) : '';
        return redirect(
          destino && destino !== '/'
            ? `${basePath}/auth/login?returnTo=${encodeURIComponent(destino)}`
            : `${basePath}/auth/login`,
        );
      }
      ctx.cookies.delete(stateCookie, clearAttrs);

      // UN FALLO AQUÍ NO PUEDE ESCAPAR COMO EXCEPCIÓN. Astro la convierte en un
      // 500 sin Content-Type ni cuerpo, y Safari no la pinta: la DESCARGA como un
      // fichero `callback`. Medido en Cast el 2026-09-28 con un secreto de cliente
      // descuadrado (`invalid_client`): el dueño veía «descargar archivo» al entrar.
      let tokens: Awaited<ReturnType<typeof exchangeCode>>;
      let claims: Awaited<ReturnType<typeof verifyIdToken>>;
      try {
        tokens = await exchangeCode(config, code, stored.codeVerifier);
        if (!tokens.idToken) return authErrorResponse('sin id_token', null);
        claims = await verifyIdToken(config, tokens.idToken, stored.nonce);
      } catch (e) {
        const motivo = e instanceof Error ? e.message : String(e);
        // `invalid_grant` es casi siempre recargar la URL de retorno o pulsar Atrás
        // (el code es de un solo uso): con el SSO vivo, reiniciar es transparente.
        if (motivo.includes('invalid_grant')) return redirect(`${basePath}/auth/login`);
        console.error(`[auth] callback: ${motivo}`);
        return authErrorResponse(motivo, null);
      }
      const session: SytueSession = {
        user: userFromClaims(claims),
        tokens,
        clientId: config.clientId,
      };
      await escribirSesion(ctx, session);
      return redirect(safeReturnTo(stored.returnTo) || `${basePath}/`);
    },

    /** POST /auth/logout — logout local + SSO (end_session) */
    async logout(ctx: BffContext): Promise<Response> {
      const session = await readSession(ctx.cookies.get(sessionCookie)?.value, sessionSecret);
      ctx.cookies.delete(sessionCookie, clearAttrs);
      clearLegacyDomainCookie(ctx);
      // Cierre LOCAL (`?scope=local`): borra solo la cookie de ESTA app y
      // termina. Lo pide Sytue ID a las diez superficies al cerrar el SSO, porque
      // una cookie host-only solo la puede borrar su propio host. Sin redirigir
      // al end_session: es el propio end_session quien esta llamando, y devolverle
      // ahi seria un bucle.
      if (ctx.url.searchParams.get('scope') === 'local') {
        return new Response(null, { status: 204 });
      }
      const doc = await discover(config.issuer, config.internalBaseUrl);
      const params = new URLSearchParams({
        client_id: config.clientId,
        post_logout_redirect_uri: config.postLogoutRedirectUri,
      });
      if (session?.tokens.idToken) params.set('id_token_hint', session.tokens.idToken);
      return redirect(`${doc.end_session_endpoint}?${params.toString()}`);
    },

    /** GET /auth/session — usuario actual (para hidratar la shell) o 401 */
    async session(ctx: BffContext): Promise<Response> {
      const session = await this.getSession(ctx);
      if (!session) return json({ authenticated: false }, 401);
      return json({ authenticated: true, user: session.user, tenant: session.user.tenant });
    },

    /**
     * GET /auth/organizations — MIS organizaciones, con cuál está activa.
     * PUT /auth/organizations — cambia la activa (body `{org_id}`).
     *
     * Vive en el BFF común y no en cada app porque el selector está en el menú
     * de cuenta de la shell, que las diez montan: si la ruta fuera de una sola
     * app, cambiar de organización solo se podría hacer desde ella.
     *
     * Tras un PUT con éxito el token que este navegador tiene sigue llevando el
     * `tenant` anterior —va firmado dentro—, así que la respuesta trae
     * `reauthorize: true` y el cliente rehace el login SSO. Con la cookie de
     * sesión de Sytue ID viva eso es una redirección, no un login.
     */
    async organizations(ctx: BffContext): Promise<Response> {
      const session = await this.getSession(ctx);
      if (!session) return json({ authenticated: false }, 401);
      const api = (config.internalBaseUrl ?? config.issuer).replace(/\/$/, '');
      const headers: Record<string, string> = {
        authorization: `Bearer ${session.tokens.accessToken}`,
        accept: 'application/json',
      };
      try {
        if (ctx.request.method === 'PUT') {
          const body = (await ctx.request.json().catch(() => null)) as { org_id?: unknown } | null;
          const orgId = typeof body?.org_id === 'string' ? body.org_id.trim() : '';
          if (!orgId) return json({ error: 'bad_request' }, 400);
          const res = await fetch(`${api}/api/v1/me/active-organization`, {
            method: 'PUT',
            headers: { ...headers, 'content-type': 'application/json' },
            body: JSON.stringify({ org_id: orgId }),
          });
          if (!res.ok) return json({ error: 'upstream' }, res.status);
          const out = (await res.json().catch(() => ({}))) as { slug?: string };
          return json({ slug: out.slug ?? '', reauthorize: true });
        }
        const res = await fetch(`${api}/api/v1/me`, { headers });
        if (!res.ok) return json({ error: 'upstream' }, res.status);
        const me = (await res.json()) as {
          active_org?: string;
          memberships?: { org_id?: string; slug?: string; name?: string; status?: string }[];
        };
        const orgs = (me.memberships ?? [])
          .filter((m) => m.status === 'active' && m.org_id && m.slug)
          .map((m) => ({
            id: String(m.org_id),
            slug: String(m.slug),
            name: m.name ? String(m.name) : String(m.slug),
            active: m.slug === me.active_org,
          }));
        return json({ organizations: orgs });
      } catch {
        // Que el IdP no conteste no puede tumbar el menú de cuenta: sin lista,
        // el selector no se pinta y todo lo demás sigue igual que antes.
        return json({ organizations: [] });
      }
    },

    /**
     * GET /auth/avatar — la foto propia, del MISMO origen que la página.
     *
     * Es la URL que lleva `user.picture` (ver `conAvatarPropio`). Pide
     * `/api/v1/me/avatar` a Sytue ID con el access token de la sesión, reenvía el
     * `If-None-Match` y devuelve su ETag, así que volver a pintar la barra es un
     * 304 sin cuerpo. Sin foto, 404: el `<Avatar>` cae a las iniciales.
     */
    async avatar(ctx: BffContext): Promise<Response> {
      const session = await this.getSession(ctx);
      if (!session) return new Response(null, { status: 401 });
      const api = (config.internalBaseUrl ?? config.issuer).replace(/\/$/, '');
      const headers: Record<string, string> = {
        authorization: `Bearer ${session.tokens.accessToken}`,
      };
      const inm = ctx.request.headers.get('if-none-match');
      if (inm) headers['if-none-match'] = inm;
      let res: Response;
      try {
        res = await fetch(`${api}/api/v1/me/avatar`, { headers });
      } catch {
        return new Response(null, { status: 502, headers: { 'cache-control': 'no-store' } });
      }
      if (res.status !== 200 && res.status !== 304) {
        // 404 es «no tiene foto», y así se cuenta; cualquier otro fallo del IdP
        // no es culpa del navegador. Nada de esto se guarda en caché.
        return new Response(null, {
          status: res.status === 404 ? 404 : 502,
          headers: { 'cache-control': 'no-store' },
        });
      }
      const out = new Headers({
        'cache-control': res.headers.get('cache-control') ?? 'private, no-cache',
        'x-content-type-options': 'nosniff',
      });
      const etag = res.headers.get('etag');
      if (etag) out.set('etag', etag);
      if (res.status === 304) return new Response(null, { status: 304, headers: out });
      out.set('content-type', res.headers.get('content-type') ?? 'application/octet-stream');
      return new Response(res.body, { status: 200, headers: out });
    },

    /** POST /auth/refresh — renueva tokens si hay refresh_token */
    async refresh(ctx: BffContext): Promise<Response> {
      const session = await readSession(ctx.cookies.get(sessionCookie)?.value, sessionSecret);
      if (!session?.tokens.refreshToken) return json({ refreshed: false }, 401);
      try {
        const tokens = await refreshTokens(config, session.tokens.refreshToken);
        const next = await nextSession(session, tokens);
        await escribirSesion(ctx, next);
        return json({ refreshed: true, expiresAt: tokens.expiresAt });
      } catch {
        ctx.cookies.delete(sessionCookie, clearAttrs);
        clearLegacyDomainCookie(ctx);
        return json({ refreshed: false }, 401);
      }
    },

    /**
     * Lee la sesion del request y la refresca de forma transparente si el
     * access token esta a punto de caducar (margen 60s). Devuelve null si no
     * hay sesion valida. Uso en middleware / guards del lado servidor.
     */
    async getSession(ctx: BffContext): Promise<SytueSession | null> {
      const session = await readSession(ctx.cookies.get(sessionCookie)?.value, sessionSecret);
      if (!session) return null;
      const aboutToExpire = session.tokens.expiresAt - Date.now() < 60_000;
      if (aboutToExpire && session.tokens.refreshToken) {
        try {
          const tokens = await refreshTokens(config, session.tokens.refreshToken);
          const next = await nextSession(session, tokens);
          await escribirSesion(ctx, next);
          return conAvatarPropio(next);
        } catch {
          return session.tokens.expiresAt > Date.now() ? conAvatarPropio(session) : null;
        }
      }
      // Al LEER y no al sellar: así también las sesiones selladas antes de esto
      // —con el `picture` del IdP dentro— salen ya con la ruta que funciona.
      return conAvatarPropio(session);
    },
  };
}

export type Bff = ReturnType<typeof createBff>;

// ---- helpers ----

function redirect(location: string): Response {
  return new Response(null, { status: 302, headers: { location } });
}

/**
 * Respuesta terminal para un error del authorization endpoint. 502: el fallo es
 * de la conversación entre el BFF y el IdP, no de la petición del navegador, y
 * un 4xx invitaría a reintentar. Se muestra el código exacto para que un
 * `invalid_scope` o un `unauthorized_client` sean diagnosticables sin HAR.
 */
function escaparHtml(t: string): string {
  return t.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}

function authErrorResponse(error: string, description: string | null): Response {
  const detail = escaparHtml(description ? `${error}: ${description}` : error);
  // HTML y no texto plano: es una página a la que se llega navegando, y con un
  // tipo que el navegador no pinta, Safari la ofrece como descarga.
  return new Response(
    '<!doctype html><html lang="es"><meta charset="utf-8">' +
      '<meta name="viewport" content="width=device-width,initial-scale=1">' +
      '<title>No se pudo iniciar sesión</title>' +
      '<body style="font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem">' +
      '<h1>No se pudo completar el inicio de sesión</h1>' +
      `<p>Sytue ID respondió: <code>${detail}</code></p>` +
      '<p>Es un problema de configuración, no de tu cuenta: reintentar no lo arregla. ' +
      'Avisa a quien administre la instalación.</p></body></html>',
    {
      status: 502,
      headers: { 'content-type': 'text/html; charset=utf-8', 'cache-control': 'no-store' },
    },
  );
}

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/**
 * Evita open-redirects: solo rutas internas absolutas. Gate unico del
 * returnTo (login -> callback) para todas las apps de la suite.
 *
 * Exige '/' inicial y veta como 2º caracter '/' y '\' (el navegador trata
 * '\' como '/', asi que '/\evil.com' seria una URL relativa a protocolo ->
 * https://evil.com/). Veta ademas cualquier control C0 en toda la cadena:
 * el parser de URL elimina \t \n \r de la Location antes de resolverla,
 * con lo que '/\t/evil.com' volveria a ser '//evil.com'.
 */
export function safeReturnTo(returnTo: string): string {
  // eslint-disable-next-line no-control-regex -- vetar C0 es el objetivo
  return /^\/(?![/\\])[^\x00-\x1f]*$/.test(returnTo) ? returnTo : '/';
}

/**
 * `returnTo` con el prefijo de la app: `/rooms` + `/?x=1` → `/rooms/?x=1`. Idempotente
 * (`/rooms`, `/rooms/…` y `/rooms?…` se quedan igual) y solo con rutas: lo que no
 * empieza por `/` lo sigue decidiendo `safeReturnTo`.
 */
export function conPrefijo(basePath: string, returnTo: string): string {
  if (!basePath || !returnTo.startsWith('/') || returnTo.startsWith('//')) return returnTo;
  const resto = returnTo.startsWith(basePath) ? returnTo.slice(basePath.length) : null;
  return resto !== null && /^(?:$|[/?#])/.test(resto) ? returnTo : `${basePath}${returnTo}`;
}
