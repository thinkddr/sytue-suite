/**
 * Sytue One — Service Worker (PWA, app-shell online-first).
 *
 * El portal es SSR y cada navegacion devuelve HTML AUTENTICADO y por-usuario, asi
 * que NUNCA se cachean navegaciones ni /auth//api (fuga de sesion / datos stale).
 * Solo se cachea el cascaron verdaderamente estatico (assets con hash de Astro,
 * iconos, manifest, pagina offline). Estrategia:
 *   - Navegacion  -> network-first, con fallback a /offline.html sin conexion.
 *   - Estaticos   -> cache-first (los bundles de Astro llevan hash: URL nueva por
 *                    build => sin stale). Version del cache para invalidar el resto.
 *   - /auth, /api -> passthrough puro (jamas se intercepta ni se cachea).
 * Offline de EDICION (registro LWW en IndexedDB + cola de ops) = Fase 2, ADR-0024.
 */

// Subir la version invalida el cache anterior (ver activate). Y hay que subirla
// SIEMPRE que cambie algo de PRECACHE: esos ficheros se sirven cache-first y el
// navegador solo reinstala el SW si cambian los bytes de ESTE fichero, asi que un
// offline.html corregido no llegaria nunca a quien ya lo tiene guardado.
// v2: offline.html en los cinco idiomas.
// v3: las rutas llevan el prefijo de la app (ver BASE).
const CACHE = 'sytue-one-shell-v3';

// EL PREFIJO SALE DE LA PROPIA URL DEL WORKER, no de una constante escrita a
// mano. Este fichero vive en `public/`, que Astro copia TAL CUAL: no le aplica
// el `base`, asi que aqui no existe `import.meta.env.BASE_URL` ni nada que Vite
// sustituya. Y el portal se sirve bajo `/one`, de modo que un `/offline.html`
// escrito en absoluto pide la raiz del host y devuelve el 404 de Traefik.
//
// El worker si sabe donde esta: se registro como `/one/sw.js`, luego su propio
// pathname menos `/sw.js` ES el prefijo. Funciona igual si algun dia la app se
// sirve en la raiz (BASE queda vacio) y no hay nada que mantener sincronizado.
const BASE = self.location.pathname.replace(/\/sw\.js$/, '');

// Cascaron minimo precacheado: pagina offline + iconos/manifest de marca.
const OFFLINE_URL = BASE + '/offline.html';
const PRECACHE = [
  OFFLINE_URL,
  BASE + '/manifest.webmanifest',
  BASE + '/icons/icon-192.png',
  BASE + '/icons/icon-512.png',
];

// Extensiones consideradas estaticas (cache-first).
const STATIC_RE =
  /\.(?:js|mjs|css|woff2?|ttf|otf|png|jpe?g|gif|svg|webp|avif|ico|json|webmanifest)$/i;

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      // addAll es atomico: si un recurso falla, no rompe la instalacion del SW.
      .then((cache) => Promise.allSettled(PRECACHE.map((u) => cache.add(u))))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return; // terceros: passthrough.

  // La ruta SIN el prefijo de la app, para comparar contra rutas escritas a mano.
  // Se comprueban las dos formas a proposito: el shim de `fetch` prefija
  // `/api/…` y `/auth/…`, pero cualquier cosa que se le escape —un `Request` ya
  // construido, una URL absoluta— llega sin prefijar. Si solo se mirara una,
  // la guarda de abajo dejaria pasar la otra.
  const rel = url.pathname.startsWith(BASE) ? url.pathname.slice(BASE.length) : url.pathname;

  // Superficie autenticada: sesion y API nunca se interceptan ni cachean.
  //
  // ESTA GUARDA ES LO QUE IMPIDE UNA FUGA, no una optimizacion: debajo hay un
  // `STATIC_RE` que cachea por extension, y `/api/algo.json` termina en `.json`.
  // Comparando solo `/api` contra el pathname completo, bajo `base` NADA casaba
  // —el pathname real es `/one/api/…`— y respuestas por-usuario habrian acabado
  // en un cache compartido por todo el que use el navegador.
  if (rel.startsWith('/auth') || rel.startsWith('/api')) return;

  // Navegaciones (HTML por-usuario): red primero, offline.html como red de seguridad.
  if (req.mode === 'navigate') {
    event.respondWith(fetch(req).catch(() => caches.match(OFFLINE_URL)));
    return;
  }

  // Estaticos con hash/estables: cache-first, poblando el cache en el primer acierto.
  if (
    rel.startsWith('/_astro/') ||
    rel.startsWith('/icons/') ||
    STATIC_RE.test(url.pathname)
  ) {
    event.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ||
          fetch(req).then((res) => {
            if (res.ok && res.type === 'basic') {
              const copy = res.clone();
              caches.open(CACHE).then((c) => c.put(req, copy));
            }
            return res;
          }),
      ),
    );
    return;
  }

  // Resto same-origin (dinamico): red directa, sin cache.
});
