/*
 * El service worker del SHELL NATIVO de Capacitor, no el de las PWA web.
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
const VERSION = 'sytue-native-v2';
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
  '/api/content', '/api/shared-content', '/api/version-content', '/api/zip', '/api/attachment',
];
const MAX_CACHE_BYTES = 50 * 1024 * 1024;
// CacheStorage is per origin. This is one shared body budget for its pages/API/assets,
// not a separate allowance for every cache/account; browser metadata is additional.
const MAX_ORIGIN_BYTES = 64 * 1024 * 1024;
const MAX_ORIGIN_ENTRIES = 1000;
const SIZE_HEADER = 'x-sytue-cache-bytes';
const TIME_HEADER = 'x-sytue-cache-written';
let epoch = 0;
let contextPromise;
let writes = Promise.resolve();
function serialWrite(work) {
  const result = writes.then(work);
  writes = result.catch(() => undefined);
  return result;
}
function retireContext() {
  epoch++;
  contextPromise = Promise.resolve(null);
}
function forgetContext() {
  retireContext();
  return serialWrite(async () => { await (await caches.open(META_CACHE)).delete(CONTEXT_URL); });
}
function background(event, work) { event.waitUntil(work.catch(() => undefined)); }
function lifetime(event) {
  let finish;
  event.waitUntil(new Promise((resolve) => { finish = resolve; }));
  return finish;
}
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => {
  event.waitUntil(caches.keys().then((keys) => Promise.all(keys
    .filter((key) => key.startsWith('sytue-native-') && !key.startsWith(VERSION + '-'))
    .map((key) => caches.delete(key))))
    .then(() => self.clients.claim()));
});
async function readContext() {
  if (!contextPromise) contextPromise = (async () => {
    const response = await (await caches.open(META_CACHE)).match(CONTEXT_URL);
    if (!response) return null;
    try {
      const value = await response.json();
      return value.origin === self.location.origin && value.ownerId && typeof value.tenantId === 'string' && value.scopeId && value.clientId ? value : null;
    } catch { return null; }
  })();
  return contextPromise;
}
async function captureScope(clientId) {
  const revision = epoch;
  const context = await readContext();
  return context && revision === epoch && (clientId === null || clientId && clientId === context.clientId) ? { context, revision } : null;
}
function current(scope) { return Boolean(scope && scope.revision === epoch); }
async function writeContext(context, revision) {
  if (revision !== epoch) return;
  const cache = await caches.open(META_CACHE);
  await cache.put(CONTEXT_URL, new Response(JSON.stringify(context), {
    headers: { 'content-type': 'application/json' },
  }));
  const keep = new Set([ownerCache('pages', context), ownerCache('api', context)]);
  for (const key of await caches.keys()) {
    if ((key.startsWith(VERSION + '-pages-') || key.startsWith(VERSION + '-api-')) && !keep.has(key)) {
      await caches.delete(key);
    }
  }
}
function ownerCache(kind, context) {
  const owner = encodeURIComponent(JSON.stringify([context.origin, context.ownerId, context.tenantId]));
  return VERSION + '-' + kind + '-' + encodeURIComponent(context.product) + '-' + owner;
}
const RE_BASE = ${RE_BASE_DE_APP.toString()};
function sinBase(pathname) { return pathname.replace(RE_BASE, ''); }
function isPrivate(pathname) {
  return PRIVATE_PREFIXES.some((prefix) => sinBase(pathname).startsWith(prefix));
}
async function cacheable(response, request, allowPrivateNoStore = false) {
  if (!response.ok || response.status !== 200 || request.headers.has('range') || response.headers.has('set-cookie')) return false;
  const url = new URL(request.url);
  const control = response.headers.get('cache-control') || '';
  const explicitlyOffline = url.searchParams.get('offline') === '1';
  const isExplicitFile = EXPLICIT_FILE_PREFIXES.some((prefix) => sinBase(url.pathname).startsWith(prefix)) ||
    (sinBase(url.pathname) === '/api/attachments' && url.searchParams.has('index'));
  if (isExplicitFile && !explicitlyOffline) return false;
  if (/no-store/i.test(control) && !explicitlyOffline && !allowPrivateNoStore) return false;
  const length = Number(response.headers.get('content-length') || '0');
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
async function budget(cacheName, request, size) {
  const entries = [];
  let bytes = size;
  for (const name of await caches.keys()) {
    if (!name.startsWith(VERSION + '-') || name === META_CACHE) continue;
    const cache = await caches.open(name);
    for (const key of await cache.keys()) {
      if (name === cacheName && key.url === request.url) continue;
      const response = await cache.match(key);
      const raw = response?.headers.get(SIZE_HEADER);
      const length = Number(raw);
      if (raw === null || !Number.isFinite(length) || length < 0) {
        await cache.delete(key); // v1/unmetered entries cannot bypass the body budget.
        continue;
      }
      bytes += length;
      entries.push({ name, key, length, time: Number(response.headers.get(TIME_HEADER) || '0') });
    }
  }
  entries.sort((a, b) => a.time - b.time || a.key.url.localeCompare(b.key.url));
  let count = entries.length + 1;
  for (const entry of entries) {
    if (bytes <= MAX_ORIGIN_BYTES && count <= MAX_ORIGIN_ENTRIES) break;
    await (await caches.open(entry.name)).delete(entry.key);
    bytes -= entry.length;
    count--;
  }
}
async function persist(cacheName, request, response, chunks, size, scope, revision) {
  await serialWrite(async () => {
    if (revision !== epoch || (scope && !current(scope)) || request.signal.aborted) return;
    await budget(cacheName, request, size);
    if (revision !== epoch || (scope && !current(scope)) || request.signal.aborted) return;
    const headers = new Headers(response.headers);
    headers.set(SIZE_HEADER, String(size));
    headers.set(TIME_HEADER, String(Date.now()));
    // fetch exposes decoded bytes. Cache metadata must describe those bytes too.
    headers.delete('content-encoding');
    headers.set('content-length', String(size));
    const cache = await caches.open(cacheName);
    await cache.put(request, new Response(new Blob(chunks), {
      status: response.status, statusText: response.statusText, headers,
    }));
    // Logout can invalidate a pending Cache.put even while it is writing.
    if (revision !== epoch || (scope && !current(scope)) || request.signal.aborted) await cache.delete(request);
  });
}
async function streamToCache(response, request, cacheName, scope, event, allowPrivateNoStore = false) {
  if (!(await cacheable(response, request, allowPrivateNoStore)) || !response.body) return response;
  const finish = lifetime(event);
  const revision = epoch;
  const reader = response.body.getReader();
  let chunks = [];
  let size = 0;
  let collecting = true;
  const declared = response.headers.get('content-length');
  const expected = declared !== null && !response.headers.has('content-encoding') ? Number(declared) : null;
  const body = new ReadableStream({
    async pull(controller) {
      try {
        const part = await reader.read();
        if (part.done) {
          reader.releaseLock();
          controller.close();
          const complete = expected === null || Number.isFinite(expected) && expected === size;
          if (collecting && complete && !request.signal.aborted && revision === epoch && (!scope || current(scope))) {
            const saved = chunks;
            chunks = [];
            void persist(cacheName, request, response, saved, size, scope, revision).catch(() => undefined).finally(finish);
          } else { chunks = []; finish(); }
          return;
        }
        size += part.value.byteLength;
        if (collecting && (size > MAX_CACHE_BYTES || request.signal.aborted || revision !== epoch || scope && !current(scope))) {
          collecting = false;
          chunks = [];
        }
        if (collecting) chunks.push(part.value.slice());
        controller.enqueue(part.value);
      } catch (error) {
        chunks = [];
        reader.releaseLock();
        controller.error(error);
        finish();
      }
    },
    async cancel(reason) {
      chunks = [];
      try { await reader.cancel(reason); } finally { reader.releaseLock(); finish(); }
    },
  });
  return new Response(body, { status: response.status, statusText: response.statusText, headers: response.headers });
}
async function precachePrivate(raw, scope, event) {
  if (!current(scope)) return;
  const request = new Request(raw, { credentials: 'include' });
  const response = await fetch(request);
  const kind = sinBase(new URL(request.url).pathname).startsWith('/api/') ? 'api' : 'pages';
  const streamed = await streamToCache(response, request, ownerCache(kind, scope.context), scope, event, true);
  // Precache has no page consumer. Drain bounded chunks, never tee/clone the network stream.
  if (streamed.body) {
    const reader = streamed.body.getReader();
    try { while (!(await reader.read()).done) { /* drain */ } } finally { reader.releaseLock(); }
  }
}
self.addEventListener('message', (event) => {
  const data = event.data || {};
  if (data.type === 'INVALIDATE') {
    background(event, (async () => {
      const context = await readContext();
      if (context?.scopeId !== data.scopeId) return;
      await forgetContext();
    })());
    return;
  }
  if (data.type !== 'INITIALISE') return;
  if (!data.ownerId || typeof data.tenantId !== 'string' || !data.scopeId || !data.product || !event.source?.id) {
    background(event, forgetContext());
    return;
  }
  const revision = ++epoch;
  const context = { clientId: event.source.id, origin: self.location.origin, ownerId: data.ownerId, tenantId: data.tenantId, scopeId: data.scopeId, product: data.product };
  contextPromise = Promise.resolve(context);
  const scope = { context, revision };
  event.waitUntil((async () => {
    await serialWrite(() => writeContext(context, revision));
    await Promise.allSettled([
      ...safeUrls(data.assetUrls).map(async (url) => {
        const request = new Request(url);
        const response = await streamToCache(await fetch(request), request, STATIC_CACHE, null, event);
        if (response.body) await response.body.pipeTo(new WritableStream());
      }),
      ...safeUrls(data.privateUrls, true).map((url) => precachePrivate(url, scope, event)),
    ]);
  })());
});
function isPublicAsset(request, url) {
  return request.destination === 'script' || request.destination === 'style' || request.destination === 'font' || sinBase(url.pathname).startsWith('/_astro/');
}
async function notify(state) {
  const clients = await self.clients.matchAll({ type: 'window' });
  clients.forEach((client) => client.postMessage({ type: 'SYTUE_NETWORK_STATE', state }));
}
async function networkFirst(request, kind, scope, event) {
  const name = ownerCache(kind, scope.context);
  let response;
  try { response = await fetch(request); }
  catch (error) {
    if (!current(scope) || request.signal.aborted) throw error;
    const cached = await (await caches.open(name)).match(request);
    if (!cached || !current(scope) || request.signal.aborted) throw error;
    background(event, notify('offline'));
    const headers = new Headers(cached.headers);
    headers.set('x-sytue-offline', 'cache');
    return new Response(cached.body, { status: cached.status, statusText: cached.statusText, headers });
  }
  background(event, notify('online'));
  return streamToCache(response, request, name, scope, event, true);
}
self.addEventListener('fetch', (event) => {
  const request = event.request;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  const path = sinBase(url.pathname);
  if (path.startsWith('/auth/logout')) {
    retireContext();
    event.respondWith(fetch(request));
    event.waitUntil((async () => {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
      windows.forEach((client) => client.postMessage({ type: 'SYTUE_CLEAR_OFFLINE' }));
      await Promise.all([
        serialWrite(async () => {
          for (const key of await caches.keys()) if (key.startsWith(VERSION + '-')) await caches.delete(key);
        }),
        new Promise((resolve) => {
          const deletion = indexedDB.deleteDatabase('sytue-offline');
          const timeout = setTimeout(() => resolve(undefined), 2000);
          deletion.onsuccess = () => { clearTimeout(timeout); resolve(undefined); };
          deletion.onerror = () => { clearTimeout(timeout); resolve(undefined); };
          deletion.onblocked = () => undefined;
        }),
      ]);
    })());
    return;
  }
  // A navigation cannot identify its new window before INITIALISE. Once a login
  // or tenant change starts, the last offline context must not stand in for it.
  // Retire the epoch before the network request and persist that invalidation;
  // keep drafts/queues and ordinary offline boot when no transition occurred.
  if (path === '/auth/login' || path === '/auth/callback' || path === '/auth/organizations' && request.method === 'PUT') {
    background(event, forgetContext());
    return;
  }
  if (request.method !== 'GET' || request.headers.has('range') || isPrivate(url.pathname)) return;
  event.respondWith((async () => {
    if (isPublicAsset(request, url)) {
      const cache = await caches.open(STATIC_CACHE);
      const hit = await cache.match(request);
      return hit || streamToCache(await fetch(request), request, STATIC_CACHE, null, event);
    }
    const scope = await captureScope(request.mode === 'navigate' ? null : event.clientId);
    if (!scope) return fetch(request);
    const kind = sinBase(url.pathname).startsWith('/api/') ? 'api' : 'pages';
    try { return await networkFirst(request, kind, scope, event); }
    catch (error) {
      if (request.mode !== 'navigate' || !current(scope) || request.signal.aborted) throw error;
      const cached = await (await caches.open(ownerCache('pages', scope.context))).match('/');
      return current(scope) ? cached || Response.error() : Response.error();
    }
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
