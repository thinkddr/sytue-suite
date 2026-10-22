/**
 * Hosts de confianza de la suite — obligatorio en TODAS las apps SSR.
 *
 * Sin esto, Astro NO se cree las cabeceras `X-Forwarded-*` (por defecto
 * `security.allowedDomains` está vacío) y, además, descarta también el `Host`
 * de la petición: `Astro.url` acaba siendo `http://localhost:<puerto>` en
 * producción, detrás del ingress.
 *
 * La consecuencia no es cosmética. `security.checkOrigin` (activo por defecto)
 * compara el `Origin` de la petición con ese origen inventado, así que TODA
 * petición que no sea GET/HEAD/OPTIONS se rechaza con 403 antes de llegar al
 * handler: el cierre de sesión por formulario y el `POST /auth/refresh` que
 * mantiene viva la sesión incluidos. Parecía un problema de sesión y era el
 * origen mal calculado.
 *
 * Se declaran los hosts, no un comodín `[{}]`: la lista existe justamente para
 * que nadie pueda mover `Astro.url` con un `X-Forwarded-Host` fabricado.
 */
export const SYTUE_ALLOWED_DOMAINS = [
  // Producción y cualquier entorno bajo el dominio de la suite.
  { hostname: '**.sytue.eu', protocol: 'https' },
  // `sytue.com` sirve LA MISMA suite (ADR 0049), así que sus peticiones tienen
  // que llegar con su `Host` intacto. Sin estas dos entradas el síntoma no es
  // un 403 evidente: `Astro.url` cae a `http://localhost:<puerto>`, el BFF se
  // cree que está en otro host —y salta al canónico, sacando a la persona del
  // dominio— y `checkOrigin` rechaza TODO POST, o sea el logout y el refresco
  // de sesión. Nada de eso falla al desplegar; falla al intentar entrar.
  //
  // El apex va aparte del comodín a propósito: `**.sytue.com` NO casa con
  // `sytue.com` a secas, que es justo el host que sirve la suite ahí.
  { hostname: 'sytue.com', protocol: 'https' },
  { hostname: '**.sytue.com', protocol: 'https' },
  // Dev local: la suite se sirve por Traefik con TLS en *.sytue.localhost.
  { hostname: '**.sytue.localhost', protocol: 'https' },
  // Acceso directo al dev-server, sin proxy delante.
  { hostname: 'localhost', protocol: 'http' },
] as const;

/**
 * Bloque `security` que consume cada `astro.config.mjs`. `checkOrigin` se deja
 * en su valor por defecto (activo): el problema nunca fue la protección CSRF,
 * sino que se estaba evaluando contra un origen que no era el real.
 */
export const sytueSecurity = {
  allowedDomains: [...SYTUE_ALLOWED_DOMAINS],
};

/**
 * Permissions-Policy de referencia: apaga las capacidades del navegador que
 * NINGUNA superficie de la suite usa. No se listan las que ya vienen acotadas a
 * `self` por defecto (portapapeles, pantalla completa…): repetir el defecto no
 * añade nada y sí invita a equivocarse.
 *
 * Rooms es la excepción y lo declara: ver `SYTUE_PERMISSIONS_POLICY_ROOMS`.
 */
export const SYTUE_PERMISSIONS_POLICY = [
  'accelerometer=()',
  'bluetooth=()',
  'camera=()',
  'display-capture=()',
  'geolocation=()',
  'gyroscope=()',
  'magnetometer=()',
  'microphone=()',
  'midi=()',
  'payment=()',
  'serial=()',
  'usb=()',
].join(', ');

/**
 * Rooms necesita cámara, micrófono y captura de pantalla para las llamadas, y
 * `autoplay` para reproducir la pista remota en cuanto llega. Se conceden SOLO
 * a su propio origen; el resto de capacidades siguen apagadas.
 */
export const SYTUE_PERMISSIONS_POLICY_ROOMS = [
  'accelerometer=()',
  'autoplay=(self)',
  'bluetooth=()',
  'camera=(self)',
  'display-capture=(self)',
  'geolocation=()',
  'gyroscope=()',
  'magnetometer=()',
  'microphone=(self)',
  'midi=()',
  'payment=()',
  'serial=()',
  'usb=()',
].join(', ');

/**
 * Cabeceras de seguridad de navegador para las superficies SSR (las nueve apps
 * autenticadas). `www` es estática y las emite su propio servidor
 * (`static-serve.mjs`).
 *
 * AQUÍ DECÍA que HSTS y `nosniff` los ponía el borde «para toda la zona», y por
 * eso no estaban. El borde NO los ponía: `sytue-edge-security` está declarado en
 * el chart y no lo engancha ningún router en el piloto, que es la producción de
 * hoy. Medido el 2026-09-21 contra una respuesta real de la app (un 404 servido
 * por su propio middleware, `one.sytue.eu/api/v1/apps`): salían CSP,
 * `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, COOP y CORP, y NO
 * salía ni `X-Content-Type-Options` ni `Strict-Transport-Security`. Las nueve
 * superficies autenticadas llevaban así desde que se escribió esta línea.
 *
 * Ahora las emiten las aplicaciones, y no porque el borde no sirva: una cabecera
 * que solo existe si un objeto de infraestructura está bien enganchado es una
 * cabecera que un día no está, y no se nota. `nosniff` cuesta un byte y cierra
 * el sniffing de MIME sobre lo que sirven los BFF y las vistas previas de Drive
 * y Photos; HSTS sobre HTTP plano lo ignora el navegador, así que ponerlo
 * siempre es seguro. `platform/pkg/httpx` y `static-serve.mjs` ya hacían esto
 * mismo: lo raro era la excepción.
 *
 * Sobre la CSP: es deliberadamente el subconjunto que NO puede romper una
 * aplicación legítima, y cada directiva cierra un ataque concreto:
 *
 *   - `frame-ancestors 'none'` (+ el `X-Frame-Options` para navegadores viejos):
 *     nadie puede meter el correo, la unidad o la consola en un iframe ajeno.
 *     Esto es lo que faltaba: sin ello, clickjacking sobre superficies con sesión.
 *   - `object-src 'none'`: sin `<object>`/`<embed>`, vector clásico de ejecución.
 *   - `base-uri 'self'`: un `<base href>` inyectado no puede reapuntar TODAS las
 *     rutas relativas a un dominio ajeno.
 *   - `form-action 'self'`: un formulario inyectado no puede enviar credenciales
 *     fuera.
 *
 * NO lleva `default-src`/`script-src`/`connect-src`: ese es el paso siguiente y
 * exige una lista por app (Rooms habla con el SFU en `wss:`, Docs incrusta
 * informes de Power BI de un origen externo, Drive sirve vistas previas desde
 * `blob:`). Ponerlo a ciegas apagaría la suite entera, así que va como ola
 * propia con su medición contra producción. ponytail: subconjunto seguro ahora;
 * `script-src`/`connect-src` cuando cada app tenga su inventario de orígenes.
 */
export const SYTUE_SECURITY_HEADERS: Readonly<Record<string, string>> = {
  'content-security-policy': [
    "object-src 'none'",
    "base-uri 'self'",
    "frame-ancestors 'none'",
    "form-action 'self'",
  ].join('; '),
  'x-content-type-options': 'nosniff',
  'strict-transport-security': 'max-age=63072000; includeSubDomains; preload',
  'x-frame-options': 'DENY',
  'referrer-policy': 'strict-origin-when-cross-origin',
  'permissions-policy': SYTUE_PERMISSIONS_POLICY,
  'cross-origin-opener-policy': 'same-origin',
  'cross-origin-resource-policy': 'same-origin',
};

/**
 * Escribe las cabeceras de seguridad sobre la respuesta ya renderizada. La usa
 * el `middleware.ts` de cada app SSR — que es el único sitio por el que pasan
 * TODAS sus rutas, incluidos los 404 y los endpoints del BFF.
 *
 * `overrides` sustituye el valor de una cabecera concreta (Rooms y su
 * Permissions-Policy). Un valor vacío la omite.
 *
 * Asume que las cabeceras de la respuesta se pueden ESCRIBIR. En Bun —que es lo
 * que ejecuta la imagen (`exec bun ./dist/server/entry.mjs`)— lo son incluso en
 * la respuesta que devuelve `fetch`, y eso importa porque varias rutas del BFF
 * hacen `return fetch(...)` del upstream tal cual (p. ej.
 * `apps/calendar/src/pages/api/calendars.ts`). Comprobado, no supuesto. Si algún
 * día el runtime pasa a Node, undici sí aplica el candado «immutable» de la
 * especificación y ahí `set()` lanzaría: entonces esto tendría que devolver una
 * Response nueva en vez de mutar la recibida.
 */
export function applySecurityHeaders(
  headers: Headers,
  overrides: Readonly<Record<string, string>> = {},
  /** Cabeceras BASE de la app, en lugar de las de la suite: la CSP cerrada de
   *  `cabecerasCerradas`. A diferencia de `overrides`, NO pisa la CSP que una ruta
   *  ya puso (el `sandbox` de un adjunto): se suma a ella, igual que la de la casa. */
  base: Readonly<Record<string, string>> = {},
): void {
  // Una `content-security-policy` que el handler YA puso en la respuesta no se
  // pisa. Varias rutas de BFF sirven un adjunto en línea (una imagen, un PDF)
  // con `content-security-policy: sandbox` para que un SVG o un HTML con un
  // `<script>` NO se ejecute al abrirlo en el origen de la app. Pisar esa
  // cabecera con la CSP de la casa —que no lleva `script-src`— volvía a dejar
  // ejecutar ese contenido: XSS almacenado. En vez de pisarla, se AÑADE la de la
  // casa, que solo endurece: el navegador aplica la intersección de todas las
  // cabeceras CSP, así que el `sandbox` de la ruta se conserva. Una CSP que el
  // middleware sustituye a propósito por `overrides` (la vista previa de Notes/
  // Photos, el editor WOPI de Docs) SÍ gana: es una decisión explícita por ruta.
  const cspPrevia = headers.get('content-security-policy');
  const hayOverrideCsp = Object.prototype.hasOwnProperty.call(overrides, 'content-security-policy');
  for (const [name, value] of Object.entries({
    ...SYTUE_SECURITY_HEADERS,
    ...base,
    ...overrides,
  })) {
    if (value === '') continue;
    if (name === 'content-security-policy' && cspPrevia && !hayOverrideCsp) {
      headers.append(name, value);
      continue;
    }
    headers.set(name, value);
  }
}
