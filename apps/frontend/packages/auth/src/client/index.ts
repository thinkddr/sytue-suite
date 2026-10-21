/**
 * @sytue/auth/client — utilidades del navegador. NUNCA maneja tokens (viven en
 * la cookie httpOnly que solo lee el BFF). Consulta /auth/session para conocer
 * al usuario.
 *
 * NO refresca la sesion por su cuenta, y no debe: eso lo hace el servidor en
 * `bff.getSession()`, que renueva los tokens cuando al access le quedan menos de
 * 60 s y solo si ALGUIEN esta pidiendo algo. Un temporizador en el navegador
 * haria el mismo trabajo a ciegas —tambien con la pestana de fondo, y una vez
 * por cada app abierta—, que es justo lo que no hace falta.
 */
import type { SytueUser } from '../types.js';

/* El lector de permisos delegados se reexporta AQUÍ además de en `/server`: es
 * puro —decodifica el payload del access token con `atob`, sin secretos ni
 * llamadas— y el conmutador de apps lo necesita en el navegador para saber si
 * pinta la tesela de Sytue Admin. No abre nada: el token ya está en ese
 * contexto, y quien decide de verdad sigue siendo el servidor. */
export { permsFromAccessToken } from '../perms.js';

export interface SessionResponse {
  authenticated: boolean;
  user?: SytueUser;
  tenant?: string;
}

const SESSION_ENDPOINT = '/auth/session';
const LOGIN_ENDPOINT = '/auth/login';
const LOGOUT_ENDPOINT = '/auth/logout';
const TOKEN_ENDPOINT = '/auth/token';

/**
 * Prefija una ruta con el base de ESTA app.
 *
 * POR QUE HACE FALTA AQUI. En su propio subdominio cada aplicacion vive en la
 * raiz, pero bajo el HOST DE ORGANIZACION —`<slug>.sytue.eu`— viven todas juntas
 * con prefijo: `/mail/...`, `/one/...`. Ahi `/auth/logout` a pelo es un 404
 * medido, y cerrar sesion no cerraba nada.
 *
 * `basePathScript` envuelve `window.fetch` y prefija lo que pase por ahi, asi que
 * `fetchSession` funcionaba de rebote. Pero `login` y
 * `logout` NAVEGAN —`location.assign`— y una navegacion no pasa por `fetch`: el
 * shim no las tocaba. Es el mismo agujero que dejo siete URL rotas en las apps.
 *
 * Se lee `window.__sytueBase` —el valor que el SERVIDOR conoce— en vez de
 * importar `withBase` de @sytue/ui: este paquete no depende de la libreria de
 * interfaz y no merece empezar a hacerlo por dos rutas. Se publica siempre, y
 * vacio significa «servida en la raiz», que es un valor y no una ausencia.
 */
function conBase(path: string): string {
  const publicado =
    typeof window !== 'undefined' ? (window as { __sytueBase?: string }).__sytueBase : undefined;
  const base = (publicado ?? '').replace(/\/+$/, '');
  // Idempotente: aplicarlo dos veces no da `/one/one/auth/logout`.
  if (!base || path === base || path.startsWith(`${base}/`)) return path;
  return `${base}${path}`;
}

/** Obtiene el usuario actual desde el BFF (o null si no hay sesion). */
export async function fetchSession(): Promise<SytueUser | null> {
  const res = await fetch(conBase(SESSION_ENDPOINT), {
    credentials: 'include',
    headers: { accept: 'application/json' },
  });
  if (!res.ok) return null;
  const data = (await res.json()) as SessionResponse;
  return data.authenticated && data.user ? data.user : null;
}

/** Redirige al login (Authorization Code + PKCE via BFF). */
export function login(returnTo: string = window.location.pathname + window.location.search): void {
  window.location.assign(`${conBase(LOGIN_ENDPOINT)}?returnTo=${encodeURIComponent(returnTo)}`);
}

/**
 * Cierra sesion en TODA la suite. NAVEGA a proposito: el BFF borra la cookie de
 * esta app y redirige al end_session de Sytue ID, que revoca la sesion SSO (y en
 * cascada los refresh tokens), pide a las demas superficies que borren la suya y
 * lleva al sitio publico. Solo el navegador puede seguir esa cadena.
 *
 * El barrido de las otras nueve cookies lo dirige SYTUE ID y no este cliente: es
 * el unico punto por el que pasan todos los cierres, incluido el que se inicia en
 * el propio IdP (el enlace de /account, que no pasa por ninguna app).
 */
const logoutCleanups = new Set<() => Promise<void>>();
/** Let clients remove device secrets before the shared SSO navigation. */
export function registerLogoutCleanup(cleanup: () => Promise<void>): () => void {
  logoutCleanups.add(cleanup);
  return () => {
    logoutCleanups.delete(cleanup);
  };
}
export function logout(): void {
  const destination = conBase(LOGOUT_ENDPOINT);
  if (logoutCleanups.size === 0) {
    window.location.assign(destination);
    return;
  }
  void Promise.allSettled([...logoutCleanups].map(async (cleanup) => cleanup())).then(() =>
    window.location.assign(destination),
  );
}

/*
 * EL ACCESS TOKEN PARA LAS ISLAS, cacheado en memoria.
 *
 * Este fichero dice arriba que el cliente «NUNCA maneja tokens», y sigue siendo
 * cierto para la sesión: el refresh vive en la cookie httpOnly que solo lee el
 * BFF. Lo que se obtiene aquí es el ACCESS token que el BFF entrega a la isla que
 * habla directamente con un servicio (JMAP, CalDAV, el WebSocket de Chat) y que
 * caduca en minutos. Nunca se persiste: si se recarga la página, se vuelve a
 * pedir.
 *
 * Vivía copiado en `src/client/token.ts` de siete apps —cinco idénticas byte a
 * byte, una con el comentario reflowado y otra con una nota más larga—, y el
 * caché en memoria es exactamente la clase de código que no se quiere tener
 * copiado: una de las copias con la holgura de expiración distinta y esa app
 * manda tokens caducados sin que nadie lo vea hasta que falla en producción.
 */
interface TokenResponse {
  accessToken: string;
  expiresAt: number;
}

let cached: TokenResponse | null = null;
let inflight: Promise<string> | null = null;

/**
 * Devuelve un access token válido, del caché si aún le quedan más de 30 segundos.
 *
 * `inflight` colapsa las peticiones simultáneas: al montar, varias islas piden
 * token a la vez y sin esto cada una abriría su propia llamada al BFF.
 *
 * Si el BFF dice que no hay sesión, NAVEGA al login y lanza: quien llama no tiene
 * nada mejor que hacer con ese error, y devolver un token vacío dejaría que la
 * llamada siguiente fallara con un 401 sin explicación.
 */
export async function getAccessToken(): Promise<string> {
  const now = Date.now();
  if (cached && cached.expiresAt - now > 30_000) return cached.accessToken;
  if (inflight) return inflight;

  inflight = (async () => {
    const res = await fetch(conBase(TOKEN_ENDPOINT), {
      credentials: 'include',
      headers: { accept: 'application/json' },
    });
    if (!res.ok) {
      // `login()` y no `location.assign('/auth/login')`: la app va bajo su prefijo
      // en el dominio de organización, así que la ruta a pelo es la raíz del HOST,
      // donde no hay ninguna app —un 404 de Traefik al caducar la sesión—. El shim
      // de `fetch` no cubre esto porque una navegación no es un fetch.
      login();
      throw new Error('No autenticado');
    }
    cached = (await res.json()) as TokenResponse;
    return cached.accessToken;
  })();

  try {
    return await inflight;
  } finally {
    inflight = null;
  }
}

export type { SytueUser } from '../types.js';
