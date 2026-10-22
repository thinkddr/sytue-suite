/*
 * El service worker del SHELL NATIVO (Wails/Capacitor), no el de las PWA web.
 *
 * ⚠️ NO ES LA FUENTE ÚNICA DE TODO, aunque lo dijera hasta el 2026-08-16 («cada
 * app expone /sw.js mediante un endpoint Astro… no hay cinco copias que puedan
 * divergir»). El reparto real, medido, es este:
 *
 * - LA WEB la sirven `portal`, `rooms` y `cloud-dashboard` con un `public/sw.js`
 *   propio y estrategias distintas. Son las TRES únicas que registran un service
 *   worker en el navegador (`swRegisterScript`), y las tres derivan su prefijo de
 *   la URL del propio worker, porque cada app va bajo `/one`, `/rooms`, `/cloud`.
 * - ESTE fichero lo sirven `drive`, `docs`, `contacts`, `mail` y `calendar` por su
 *   `sw.js.ts`, y solo para el SHELL NATIVO.
 *
 *   Aquí decía «que corre desde la raíz. Por eso compara rutas absolutas
 *   (`/api/`, `/_astro/`, `isPrivate`) sin prefijo: ahí no lo hay», y AVISABA de
 *   que servirlo bajo un `base` «rompería `isPrivate` en silencio y se cachearía
 *   lo privado». Era falso, y eso es exactamente lo que estaba pasando:
 *   `productURL` (del shell de Wails, retirado el 2026-09-23) abría la ventana en
 *   `https://mail.sytue.eu/`, y esa raíz responde 301 a `/mail` — medido en las
 *   cinco. Dentro de la ventana nativa las rutas SÍ llevan prefijo, así que
 *   ninguna de las comparaciones casaba: lo privado era cacheable y, sobre todo,
 *   la purga de cerrar sesión no se ejecutaba nunca.
 *
 *   Ahora todas pasan por `sinBase()`, así que da igual con prefijo o sin él.
 *
 * `rooms` tenía las dos cosas y su estático TAPABA al endpoint: ese `sw.js.ts`
 * muerto se borró el 2026-08-17. Consecuencia explícita, por si algún día hace
 * falta: el shell nativo de rooms usa el worker de PWA, no este; para cambiarlo
 * habría que quitar su `public/sw.js`, no volver a añadir el endpoint.
 *
 * Unificar los tres estáticos con este sigue pendiente y cambia la política de
 * caché de tres aplicaciones a la vez: primero el barrido en verde, para notarlo.
 */

/**
 * El PREFIJO DE BASE de la app, cuando lo hay.
 *
 * Se creía que el shell nativo corría desde la raíz —lo decía la cabecera de
 * este fichero— y por eso el worker comparaba rutas absolutas. No es cierto:
 * `productURL` (del shell de Wails, ya retirado) cargaba `https://mail.sytue.eu/`, y
 * esa raíz responde 301 a `/mail`. Comprobado en las cinco apps que sirven este
 * worker: mail, drive, docs, contacts y calendar redirigen todas a su prefijo.
 * O sea que dentro de la ventana nativa las rutas son `/mail/api/…`,
 * `/mail/auth/…`, y NINGUNA de las comparaciones casaba.
 *
 * Solo se recorta un segmento y solo si detrás viene una raíz interna conocida,
 * para no tocar rutas de contenido: `/docs/edit` se queda como está.
 */
export const RE_BASE_DE_APP = /^\/[a-z-]+(?=\/(?:auth|api|_astro)\/)/;

const SOURCE = String.raw`
const VERSION = 'sytue-native-v1';
const STATIC_CACHE = VERSION + '-static';
const META_CACHE = VERSION + '-meta';
const CONTEXT_URL = '/__sytue_offline_context__';
const PRIVATE_PREFIXES = [
  '/auth/',
  '/api/token',
  '/api/session',
  '/api/intent',
  '/api/presign',
  '/api/ws-ticket',
  '/api/collab',
  '/api/call',
  '/api/conference',
  '/api/ai',
];
const EXPLICIT_FILE_PREFIXES = [
  '/api/content',
  '/api/shared-content',
  '/api/version-content',
  '/api/zip',
  '/api/attachment',
];
const MAX_CACHE_BYTES = 50 * 1024 * 1024;

self.addEventListener('install', () => self.skipWaiting());

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(
        keys.filter((key) =>
          key === STATIC_CACHE ||
          (key.startsWith('sytue-native-') && !key.startsWith(VERSION)),
        )
          .map((key) => caches.delete(key)),
      ))
      .then(() => self.clients.claim()),
  );
});

async function readContext() {
  const cache = await caches.open(META_CACHE);
  const response = await cache.match(CONTEXT_URL);
  if (!response) return null;
  try { return await response.json(); } catch { return null; }
}

async function writeContext(context) {
  const cache = await caches.open(META_CACHE);
  await cache.put(
    CONTEXT_URL,
    new Response(JSON.stringify(context), { headers: { 'content-type': 'application/json' } }),
  );
  const keep = new Set([ownerCache('pages', context), ownerCache('api', context)]);
  const keys = await caches.keys();
  await Promise.all(keys
    .filter((key) =>
      (key.startsWith(VERSION + '-pages-') || key.startsWith(VERSION + '-api-')) &&
      !keep.has(key),
    )
    .map((key) => caches.delete(key)));
}

function ownerCache(kind, context) {
  const owner = encodeURIComponent(String(context.ownerId || 'anonymous'));
  const product = encodeURIComponent(String(context.product || 'suite'));
  return VERSION + '-' + kind + '-' + product + '-' + owner;
}

// La ruta SIN el prefijo de base de la app (ver RE_BASE_DE_APP). El literal sale
// de la constante exportada para que el test pueda comprobar la MISMA expresión
// que corre aquí dentro, y no una copia que diverja.
const RE_BASE = ${RE_BASE_DE_APP.toString()};
function sinBase(pathname) {
  return pathname.replace(RE_BASE, '');
}

function isPrivate(pathname) {
  const ruta = sinBase(pathname);
  return PRIVATE_PREFIXES.some((prefix) => ruta.startsWith(prefix));
}

async function cacheable(response, request, allowPrivateNoStore = false) {
  if (!response.ok || response.status !== 200 || request.headers.has('range')) return false;
  const url = new URL(request.url);
  const control = response.headers.get('cache-control') || '';
  const explicitlyOffline = url.searchParams.get('offline') === '1';
  const isExplicitFile =
    EXPLICIT_FILE_PREFIXES.some((prefix) => sinBase(url.pathname).startsWith(prefix)) ||
    (sinBase(url.pathname) === '/api/attachments' && url.searchParams.has('index'));
  if (isExplicitFile && !explicitlyOffline) return false;
  if (/no-store/i.test(control) && !explicitlyOffline && !allowPrivateNoStore) return false;
  const length = Number(response.headers.get('content-length') || '0');
  // Un fichero marcado para uso offline sin tamaño declarado no se cachea: permitirlo
  // haría ficticio el límite y una respuesta chunked podría llenar el dispositivo.
  if (explicitlyOffline && (!Number.isFinite(length) || length <= 0)) return false;
  return !Number.isFinite(length) || length === 0 || length <= MAX_CACHE_BYTES;
}

function safeUrls(values, includeApi = false) {
  if (!Array.isArray(values)) return [];
  return values.filter((raw) => {
    try {
      const url = new URL(raw, self.location.origin);
      return url.origin === self.location.origin && !isPrivate(url.pathname) && (includeApi || !sinBase(url.pathname).startsWith('/api/'));
    } catch { return false; }
  });
}

async function precachePrivate(raw, context) {
  const request = new Request(raw, { credentials: 'include' });
  const response = await fetch(request);
  if (!(await cacheable(response, request, true))) return;
  const url = new URL(request.url);
  const kind = sinBase(url.pathname).startsWith('/api/') ? 'api' : 'pages';
  const cache = await caches.open(ownerCache(kind, context));
  await cache.put(request, response);
}

self.addEventListener('message', (event) => {
  const data = event.data || {};
  if (data.type !== 'INITIALISE' || !data.ownerId || !data.product) return;
  const context = { ownerId: data.ownerId, product: data.product };
  const assetUrls = safeUrls(data.assetUrls);
  const privateUrls = safeUrls(data.privateUrls, true);
  event.waitUntil(
    (async () => {
      await writeContext(context);
      const assets = await caches.open(STATIC_CACHE);
      await Promise.all([
        Promise.allSettled(assetUrls.map((url) => assets.add(url))),
        Promise.allSettled(privateUrls.map((url) => precachePrivate(url, context))),
      ]);
    })(),
  );
});

function isPublicAsset(request, url) {
  return (
    request.destination === 'script' ||
    request.destination === 'style' ||
    request.destination === 'font' ||
    sinBase(url.pathname).startsWith('/_astro/')
  );
}

async function networkFirst(request, cacheName) {
  const cache = await caches.open(cacheName);
  try {
    const response = await fetch(request);
    const clients = await self.clients.matchAll({ type: 'window' });
    clients.forEach((client) => client.postMessage({ type: 'SYTUE_NETWORK_STATE', state: 'online' }));
    if (await cacheable(response, request, true)) await cache.put(request, response.clone());
    return response;
  } catch (error) {
    const cached = await cache.match(request);
    if (cached) {
      const clients = await self.clients.matchAll({ type: 'window' });
      clients.forEach((client) => client.postMessage({ type: 'SYTUE_NETWORK_STATE', state: 'offline' }));
      const headers = new Headers(cached.headers);
      headers.set('x-sytue-offline', 'cache');
      return new Response(cached.body, {
        status: cached.status,
        statusText: cached.statusText,
        headers,
      });
    }
    throw error;
  }
}

self.addEventListener('fetch', (event) => {
  const request = event.request;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  if (sinBase(url.pathname).startsWith('/auth/logout')) {
    event.respondWith(fetch(request));
    event.waitUntil(
      (async () => {
        const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
        windows.forEach((client) => client.postMessage({ type: 'SYTUE_CLEAR_OFFLINE' }));
        await Promise.all([
          caches.keys().then((keys) => Promise.all(
            keys.filter((key) => key.startsWith(VERSION + '-')).map((key) => caches.delete(key)),
          )),
          // Las ventanas cierran primero su conexión. El timeout evita retener
          // indefinidamente el worker si una pestaña congelada no responde.
          new Promise((resolve) => {
            const deletion = indexedDB.deleteDatabase('sytue-offline');
            const timeout = setTimeout(() => resolve(undefined), 2000);
            deletion.onsuccess = () => { clearTimeout(timeout); resolve(undefined); };
            deletion.onerror = () => { clearTimeout(timeout); resolve(undefined); };
            deletion.onblocked = () => undefined;
          }),
        ]);
      })(),
    );
    return;
  }
  if (request.method !== 'GET' || isPrivate(url.pathname)) return;

  if (request.mode === 'navigate') {
    event.respondWith((async () => {
      const context = await readContext();
      if (!context) return fetch(request);
      try {
        return await networkFirst(request, ownerCache('pages', context));
      } catch {
        const cache = await caches.open(ownerCache('pages', context));
        return (await cache.match('/')) || Response.error();
      }
    })());
    return;
  }

  if (sinBase(url.pathname).startsWith('/api/')) {
    event.respondWith((async () => {
      const context = await readContext();
      if (!context) return fetch(request);
      return networkFirst(request, ownerCache('api', context));
    })());
    return;
  }

  if (isPublicAsset(request, url)) {
    event.respondWith(
      caches.open(STATIC_CACHE).then((cache) =>
        cache.match(request).then((cached) => cached || fetch(request).then(async (response) => {
          if (await cacheable(response, request)) await cache.put(request, response.clone());
          return response;
        })),
      ),
    );
    return;
  }

  event.respondWith((async () => {
    const context = await readContext();
    if (!context) return fetch(request);
    return networkFirst(request, ownerCache('pages', context));
  })());
});
`;

export function nativeWorkerResponse(): Response {
  return new Response(SOURCE, {
    headers: {
      'content-type': 'text/javascript; charset=utf-8',
      'cache-control': 'no-cache, no-store, must-revalidate',
      'service-worker-allowed': '/',
    },
  });
}
