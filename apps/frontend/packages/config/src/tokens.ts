/**
 * Sytue One — tokens de diseño en TypeScript.
 *
 * Valores canónicos de branding/tokens/*.json, vendorizados por
 * scripts/sync-branding.sh para el contexto de build del frontend.
 */
import colors from './tokens/colors.json';
import spacing from './tokens/spacing.json';

type Scale<T> = {
  readonly [
    K in keyof T as K extends `${infer N extends number}` ? N : K extends number ? K : never
  ]: T[K];
};
type Values<T> = { readonly [K in keyof T as K extends `$${string}` ? never : K]: T[K] };

/** Solo los escalones numéricos, sin los roles ni las notas del catálogo. */
function scale<T extends object>(tokens: T): Scale<T> {
  return Object.fromEntries(
    Object.entries(tokens).filter(([key]) => /^\d+$/.test(key)),
  ) as Scale<T>;
}

/** Las notas describen el catálogo; no forman parte de la API de tokens. */
function values<T extends object>(tokens: T): Values<T> {
  return Object.fromEntries(
    Object.entries(tokens).filter(([key]) => !key.startsWith('$')),
  ) as Values<T>;
}

export const brand = {
  primary: scale(colors.primary),
  copper: scale(colors.copper),
  accent: scale(colors.accent),
  violet: scale(colors.violet),
  neutral: scale(colors.neutral),
} as const;

function semanticRole(token: {
  base: string;
  hover: string;
  text: string;
  bg: string;
  on: string;
}) {
  return {
    base: token.base,
    hover: token.hover,
    text: token.text,
    bg: token.bg,
    on: token.on,
  } as const;
}

export const semantic = {
  success: semanticRole(colors.semantic.success),
  warning: semanticRole(colors.semantic.warning),
  danger: semanticRole(colors.semantic.danger),
  info: semanticRole(colors.semantic.info),
} as const;

export type PresenceStatus = 'online' | 'away' | 'dnd' | 'offline';
export const presence: Record<PresenceStatus, string> = values(colors.presence);

/** Identidad por producto; las claves legadas conservan su color canónico. */
export const productColor = values(colors.product);

/** Roles por tema para muestras que deben enseñar ambos modos a la vez. */
export const themeColors = colors.theme;

export type ProductKey = keyof typeof productColor;

/**
 * Productos que solo existen para organizaciones: una cuenta personal no los abre, pague
 * o no (ADR 0038 para Sytue ERP; decisión 17 del dueño para Sytue Web). Es UNA lista
 * para los cuatro sitios que la aplican —el conmutador, el lanzador del portal, el
 * sitio público y el Hub nativo, que la lee de aquí en un test—: con una copia en cada
 * uno, el día que entró Pages se habría quedado abierto en alguno.
 *
 * Vive aquí, junto al resto de lo que se sabe de cada clave de producto, y no en
 * `@sytue/auth`, porque el sitio público no depende de la sesión; la regla con la
 * sesión delante es `soloOrganizacion(key, user)` de `@sytue/auth`.
 */
export const PRODUCTOS_SOLO_ORGANIZACION = [
  'erp',
  'pages',
  'bases',
] as const satisfies readonly ProductKey[];

/** Si `key` es un producto solo para organizaciones. No mira quién pregunta. */
export function esSoloDeOrganizaciones(key: string): boolean {
  return (PRODUCTOS_SOLO_ORGANIZACION as readonly string[]).includes(key);
}

export const radii = values(spacing.radii);
export const layout = values(spacing.layout);
export const zIndex = { ...spacing.zIndex } as const;
export const motion = {
  duration: { ...spacing.motion.duration },
  easing: { ...spacing.motion.easing },
} as const;

export const fontFamily = {
  sans: '"Figtree Variable", "Inter Variable", "Segoe UI", system-ui, -apple-system, BlinkMacSystemFont, "Helvetica Neue", Arial, sans-serif',
  mono: '"JetBrains Mono", "SFMono-Regular", ui-monospace, "Cascadia Code", "Roboto Mono", Menlo, Consolas, monospace',
} as const;

// ponytail: apex del entorno — dev `sytue.localhost`, prod `sytue.eu`. Se deriva del
// issuer OIDC (PUBLIC_OIDC_ISSUER, p. ej. https://id.sytue.localhost) que compose
// inyecta y Vite hornea IGUAL en SSR y cliente, así toda la suite emite enlaces
// coherentes sin depender de `location` (undefined en SSR -> antes horneaba prod).
function resolveSuiteApex(): string {
  // https://id.<apex> -> <apex>: host del issuer sin su primera etiqueta DNS.
  const issuer = (import.meta as { env?: { PUBLIC_OIDC_ISSUER?: string } }).env?.PUBLIC_OIDC_ISSUER;
  if (issuer) {
    try {
      const apex = new URL(issuer).hostname.split('.').slice(1).join('.');
      if (apex) return apex;
    } catch {
      /* issuer mal formado -> cae al fallback de abajo */
    }
  }
  // Sin issuer inyectado (import fuera de Vite): rescata por host en el navegador.
  const host = (globalThis as { location?: { hostname: string } }).location?.hostname;
  return host?.endsWith('.sytue.localhost') ? 'sytue.localhost' : 'sytue.eu';
}

export const suiteApex: string = resolveSuiteApex();

/**
 * Cookie con el color de la organización activa (ADR 0071). La escribe el BFF junto
 * a la sesión y la lee el script del primer pintado de `@sytue/ui`, que no puede
 * importar nada: por eso allí el nombre está repetido, y `init.test.ts` los compara.
 */
export const ORG_COLOR_COOKIE = 'sytue-color-org';
/** Y la de su forma («Marco» o «Carril»): el mismo camino y el mismo motivo. */
export const ORG_FORMA_COOKIE = 'sytue-forma-org';

/** Subdominios canónicos de la suite en el entorno actual (ver CONVENTIONS §1). */
const at = (sub: string): string => `https://${sub}.${suiteApex}`;
export const appDomains = {
  one: at('one'),
  mail: at('mail'),
  drive: at('drive'),
  calendar: at('calendar'),
  contacts: at('contacts'),
  // La ofimática: una app por producto desde el 2026-10-12 (ADR 0076), cada una con su
  // host. Las claves siguen siendo las de siempre —`docs` es Docus, `sheets` Calcus,
  // `slides` Diapos y `pdf` CertiPDF— y `docs.<dominio>` queda solo para la API del
  // servicio de documentos: nadie construye ya una URL web con él.
  docs: at('docus'),
  notes: at('notes'),
  photos: at('photos'),
  sheets: at('calcus'),
  slides: at('diapos'),
  pdf: at('certipdf'),
  // Sytue Rooms (Chat+Meet unificados) tiene HOST PROPIO. `chat` y `meet` quedan
  // como claves LEGADAS que apuntan ya a Rooms: sus hosts responden 301 a rooms.*
  // (Traefik), así que nadie debe construir URLs nuevas con ellas.
  chat: at('rooms'),
  meet: at('rooms'),
  rooms: at('rooms'),
  cloud: at('cloud'),
  // Sytue Admin (antes «Console») tiene HOST PROPIO admin.*; console.* responde
  // 301 a él (Traefik), así que la clave `console` —que sigue siendo la del
  // producto en el código— ya apunta al host nuevo. Mismo caso que chat/meet.
  console: at('admin'),
  hub: at('hub'),
  erp: at('erp'),
  cast: at('cast'),
  // Sytue Web (antes «Sytue Pages») tiene HOST `web.*` desde el 2026-10-04. La clave
  // del producto sigue siendo `pages` —carpetas, cliente OIDC, bases, token de color—;
  // lo que cambia es la dirección. `pages.*` sigue sirviendo la app hasta que el
  // despliegue lo convierta en redirección (`redirigirHostAnterior` en el chart de
  // frontends, apagado por defecto): nadie debe construir URLs nuevas con él.
  pages: at('web'),
  bases: at('bases'),
  id: at('id'),
} satisfies Record<ProductKey, string>;

/**
 * Prefijo bajo el que cada superficie se sirve dentro de su host.
 *
 * NO es una decoración: `astro.config` lo fija con `base` y el chart del
 * frontend con `path`, así que la página de la consola vive en `/admin/apps`, no
 * en `/apps`. Traefik solo redirige la RAÍZ al prefijo (middleware
 * `raiz-al-prefijo`, regex `^https?://([^/]+)/?(\?.*)?$`), de modo que las teselas del
 * conmutador funcionan y cualquier enlace PROFUNDO escrito sin prefijo da 404.
 *
 * Sytue ID no está aquí a propósito: lo sirve un binario Go, no Astro, y sus
 * rutas (`/account`, `/branding/...`) cuelgan de la raíz.
 */
const appPaths: Partial<Record<ProductKey, string>> = {
  one: '/one',
  mail: '/mail',
  drive: '/drive',
  calendar: '/calendar',
  contacts: '/contacts',
  docs: '/docus',
  sheets: '/calcus',
  slides: '/diapos',
  pdf: '/certipdf',
  // Notepads también: su `astro.config` y el chart la sirven bajo `/notes`. Faltaba,
  // y lo tapaba que la raíz exacta sí llega al pod —el `?notebook=…` de la búsqueda
  // funcionaba de rebote—; lo profundo, como el `/p/{token}` de un cuaderno
  // publicado, era el 404 de Traefik.
  notes: '/notes',
  photos: '/photos',
  chat: '/rooms',
  meet: '/rooms',
  rooms: '/rooms',
  cloud: '/cloud',
  console: '/admin',
  erp: '/erp',
  cast: '/cast',
  // Sytue Web: `/web` desde el 2026-10-04. `/pages` fue su prefijo y el chart lo
  // redirige a este en cualquier host (`pathAnterior`).
  pages: '/web',
  bases: '/bases',
};

/**
 * El prefijo de path bajo el que se sirve una app dentro de su host
 * (`/mail`, `/calcus`), o null si su host NO tiene prefijo (`id`, `hub`): sus
 * rutas cuelgan de la raíz y ahí no hay nada tras lo que escopiar una
 * organización (ADR 0056). Lo lee quien compone enlaces con org y necesita
 * saber si el destino la admite —y quién quiere el base de la app ACTUAL para
 * `orgActual(base)`—. `appUrl` ya lo aplica solo; este accessor existe para no
 * duplicar la tabla ni exportarla mutable.
 */
export function prefijoDeApp(key: ProductKey): string | null {
  return appPaths[key] ?? null;
}

/**
 * URL pública de una página de OTRA app de la suite.
 *
 * Se pasa la ruta tal y como se escribe dentro de esa app —sin su prefijo—, que
 * es justo lo que uno tiende a escribir. El fallo que evita no se parece a un
 * enlace roto: el host resuelve, el Ingress enruta al pod y lo que contesta es
 * el 404 de Astro, así que se lee como «esa app está caída».
 *
 * Con `org`, la URL lleva la organización como segmento de path
 * (`calcus.sytue.eu/calcus/<org>/edit?id=…`, ADR 0056): es la forma canónica y
 * aterriza sin redirección. Sin org también funciona —el middleware de la app de
 * destino la añade con la org de la sesión—, pero cuesta un salto. Acepta null
 * a propósito: es lo que devuelve `orgActual(base)` cuando la URL actual no la
 * lleva, y componer `appUrl(key, path, orgActual(base))` no debe necesitar
 * coerción ninguna.
 */
export function appUrl(key: ProductKey, path = '', org?: string | null): string {
  const orgSeg = org ? `/${org}` : '';
  const base = `${appDomains[key]}${appPaths[key] ?? ''}${orgSeg}`;
  const limpia = path.replace(/^\/+/, '');
  return limpia ? `${base}/${limpia}` : base;
}

/**
 * El enlace de una reunión de Sytue Rooms: `https://rooms.<dominio>/?rooms=<id>`, en el
 * host PROPIO de Rooms y en su raíz (decidido por el dueño el 2026-09-25). Es el que se
 * copia, el que Calendar guarda en CONFERENCE y el que viaja por correo: un invitado
 * no tiene por qué ser de la organización, así que no puede ser el host de esta.
 *
 * `meet` fue el parámetro anterior y NO se genera más; solo se sigue leyendo como
 * alias (`pages/index.astro` de Rooms lo redirige). Lo vigila
 * `test/gates/enlace-de-reunion.test.ts`.
 *
 * `clave` es la del enlace de una reunión suelta (decidido por el dueño el 2026-10-16:
 * «Invitados y quien tenga el enlace»): sin ella solo entran quien la convocó y quien
 * ya estuvo dentro. Se pide siempre para que nadie la olvide al componer un enlace.
 */
export function enlaceReunion(id: string, clave: string, host: string = appDomains.rooms): string {
  const base = `${host.replace(/\/+$/, '')}/?rooms=${encodeURIComponent(id)}`;
  return clave ? `${base}&clave=${encodeURIComponent(clave)}` : base;
}

/** Atajo para Sytue Admin, que es a donde más se enlaza desde otras apps. */
export function adminUrl(path = ''): string {
  return appUrl('console', path);
}

/**
 * Vuelta a una CARPETA de Sytue Drive.
 *
 * Es el enlace con el que las apps que abren un fichero de Drive —CertiPDF con
 * un PDF, Fotos con una imagen— devuelven a donde la persona lo pulsó. Va aquí y
 * no en cada app porque la convención del parámetro (`?path=`) es una sola, y
 * escrita tres veces se desalinea a la primera.
 *
 * La raíz se pasa SIN parámetro: `?path=/` funciona, pero deja una URL que
 * invita a pensar que hay una carpeta llamada «/».
 *
 * Con `org` (ADR 0056) la vuelta sale canónica y aterriza sin el 302 que de
 * otro modo añade el middleware de Drive. Sin org también vale: el enlace sirve
 * igual para quien no lleva la organización en la URL actual.
 */
export function driveFolderUrl(path: string, org?: string | null): string {
  return appUrl('drive', path && path !== '/' ? `?path=${encodeURIComponent(path)}` : '', org);
}

/**
 * El enlace canónico de un FICHERO de Sytue Drive (`/drive/files/{id}`): el que
 * usan la búsqueda federada y el Hub, y el que se guarda en una nota, se adjunta
 * a un correo o se publica en Rooms. Es el `EnlaceFichero` de la app nativa.
 * Escrito a mano, cada sitio lo dejaba sin el prefijo, y eso es un 404.
 */
export function driveFileUrl(id: string): string {
  return appUrl('drive', `/files/${encodeURIComponent(id)}`);
}

/**
 * El producto de la ofimática que abre cada tipo de documento: desde el 2026-10-12 cada
 * uno tiene su app y su host (ADR 0076), así que quien enlaza un documento tiene que
 * saber su tipo para no mandarlo a `docus.*` siendo una hoja.
 */
export const PRODUCTO_DE_TIPO = {
  document: 'docs',
  spreadsheet: 'sheets',
  presentation: 'slides',
} as const satisfies Record<string, ProductKey>;

/**
 * El editor de un documento de la ofimática (`calcus.sytue.eu/calcus/<org>/edit?id=…`).
 * Sin tipo conocido va a Docus, que manda el documento a la app de su producto: el
 * enlace no se rompe, solo cuesta un salto.
 */
export function enlaceDocumento(
  id: string,
  kind: string | null | undefined,
  org?: string | null,
): string {
  const producto = PRODUCTO_DE_TIPO[kind as keyof typeof PRODUCTO_DE_TIPO] ?? 'docs';
  return appUrl(producto, `/edit?id=${encodeURIComponent(id)}`, org);
}

/**
 * La carpeta de Drive que trae un `?volver=`, o undefined si no la trae.
 *
 * ES UNA VERJA, no una comodidad. Lo que llega por la query lo elige quien
 * compone el enlace, así que admitir una URL completa convertiría a la app que
 * abre el fichero en un REDIRECTOR ABIERTO: bastaría con mandar a alguien a
 * `…/pdf?path=x&volver=https://sitio-de-quien-sea` para que la vuelta —con el
 * aspecto y el dominio de la suite— llevase fuera. Solo se acepta una RUTA
 * ABSOLUTA de Drive; el destino lo compone `driveFolderUrl` con el host nuestro.
 *
 * `//otro-sitio` es una URL con esquema heredado y empieza por «/»: por eso no
 * basta con mirar el primer carácter.
 */
export function volverA(valor: string | null | undefined): string | undefined {
  if (!valor || !valor.startsWith('/') || valor.startsWith('//')) return undefined;
  return valor;
}

/** Sytue Integration Gateway. NO es un ProductKey —no tiene tesela en el conmutador ni
 *  aparece en el portal— pero sí host propio: la shell le pregunta quién sirve cada
 *  producto y la consola gestiona las conexiones. Va aparte para no ensanchar
 *  `appDomains`, cuyo tipo garantiza que están TODOS los productos y solo ellos. */
export const integrationsDomain: string = at('integrations');

/**
 * Los orígenes AJENOS al de la app con los que habla la shell común (`@sytue/shell`)
 * desde el navegador. Los necesita la app que cierre `connect-src` o `img-src` en su
 * CSP: si no los deja pasar, la shell se queda muda SIN un error a la vista —la campana
 * no trae avisos, no hay presencia, el conmutador no sabe qué hay desplegado— y solo se
 * ve en la consola del navegador. Así estuvo Sytue Web desde su alta hasta el 2026-10-05,
 * con `connect-src 'self'`.
 *
 *   - `conectar`: el Hub (avisos, su flujo SSE, presencia, búsqueda y `/api/v1/apps`) y
 *     el gateway de integraciones (`useProductMap`: quién sirve cada producto).
 *   - `imagenes`: Sytue ID, que sirve el logo de la organización del conmutador
 *     (`/branding/<org>/logo.png`). El avatar NO: va por `/auth/avatar`, mismo origen.
 *
 * `hubBaseUrl` es el MISMO valor que la app le pasa a `<SytueShell hubBaseUrl>`; sin él,
 * el que la shell usa por defecto. Vive aquí, junto a los dominios, y no en cada app:
 * quien añada a la shell una llamada a otro origen la añade en este sitio y todas las
 * CSP cerradas se enteran. Una URL mal formada se omite en vez de lanzar: esto corre en
 * el middleware de cada petición, y una campana muda es mejor que un 500 en todo.
 */
export function origenesDeLaShell(hubBaseUrl: string = appDomains.hub): {
  readonly conectar: readonly string[];
  readonly imagenes: readonly string[];
} {
  const origen = (url: string): string[] => {
    try {
      return [new URL(url).origin];
    } catch {
      return [];
    }
  };
  return {
    conectar: [...origen(hubBaseUrl), ...origen(integrationsDomain)],
    imagenes: origen(appDomains.id),
  };
}
