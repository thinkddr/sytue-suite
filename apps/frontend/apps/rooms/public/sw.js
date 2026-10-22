/*
 * Service Worker de Sytue Rooms (Ola 18 · PWA + Web Push).
 *
 * - App-shell: los assets INMUTABLES de Astro (/_astro/…) y el branding se sirven
 *   cache-first (hash en el nombre ⇒ nunca stale). Las navegaciones van network-first
 *   con un fallback offline mínimo. La API/BFF (/api/, /auth/) NO se cachea (respuestas
 *   por-usuario y autenticadas): pasa directa a la red — «network-first» degenerado a
 *   network-only, seguro para datos privados.
 * - Web Push (RFC 8291): el navegador descifra el payload y aquí se muestra la
 *   notificación (salvo que la app esté abierta: entonces la propia página avisa in-app,
 *   evitando duplicar). Al pulsarla se enfoca/abre el canal (deep-link).
 * - Actualización controlada: skipWaiting + clients.claim para activar la versión nueva
 *   sin dejar dos SW conviviendo; al activar se purgan las cachés viejas.
 *
 * Servido estáticamente en /sw.js (scope «/»). No interviene en el SSR de Astro.
 */

const VERSION = 'v2';
const STATIC = `sytue-rooms-static-${VERSION}`;

// EL PREFIJO SALE DE LA PROPIA URL DEL WORKER. Este fichero vive en `public/`,
// que Astro copia TAL CUAL: no le aplica el `base`, así que aquí no hay nada que
// Vite sustituya. Y Rooms se sirve bajo `/rooms`, de modo que las rutas escritas
// en absoluto pedían la raíz del host y devolvían el 404 de Traefik.
//
// Con `addAll` eso no era un fallo parcial: es ATÓMICO, así que los tres 404
// rechazaban la promesa y **el service worker no llegaba a instalarse**. Sin
// worker no hay caché ni página offline: la función entera estaba muerta y el
// único rastro eran tres peticiones rojas en la pestaña de red.
//
// El worker sí sabe dónde está: se registró como `/rooms/sw.js`, luego su propio
// pathname menos `/sw.js` ES el prefijo. Si algún día se sirve en la raíz, queda
// vacío y todo sigue igual.
const BASE = self.location.pathname.replace(/\/sw\.js$/, '');

// Un asset inmutable de Astro, mire quien lo mire: con prefijo o sin él.
function esAsset(pathname) {
  const rel = pathname.startsWith(BASE) ? pathname.slice(BASE.length) : pathname;
  return rel.startsWith('/_astro/');
}

// Assets a precachear en la instalación (branding + manifest). Los /_astro/ hashed se
// cachean bajo demanda (cache-first) la primera vez que se piden.
const PRECACHE = [
  `${BASE}/favicon.svg`,
  `${BASE}/icon-maskable.svg`,
  `${BASE}/manifest.webmanifest`,
];

// Página offline mínima (inline: sin depender de una ruta SSR). Paleta Graphite & Brass.
//
// Los CINCO idiomas, como el resto de la plataforma. Estaba entera en castellano y con
// `lang="es"` fijo, y es la única pantalla que ve quien se queda sin red: quien navega
// en inglés, francés, alemán o italiano recibía un idioma que no es el suyo justo en el
// aviso que no puede reintentar. El inglés va en el HTML —si el script no llega a
// correr, se lee igual— y el resto se aplica con la MISMA regla que el middleware
// (cookie explícita > idioma del navegador). Mismo patrón que apps/portal/public/offline.html,
// pero en línea: así viaja dentro del propio sw.js, que el navegador revalida byte a byte,
// y no hace falta subir la versión de la caché ni tocar PRECACHE (un fichero precacheado
// se sirve cache-first y la corrección no llegaría nunca a quien ya lo tiene).
const OFFLINE_TEXTS = {
  es: [
    'Sin conexión · Sytue Rooms',
    'Sin conexión',
    'Sytue Rooms necesita conexión para cargar. Vuelve a intentarlo cuando tengas red.',
  ],
  fr: [
    'Pas de connexion · Sytue Rooms',
    'Pas de connexion',
    'Sytue Rooms a besoin d’une connexion pour se charger. Réessayez une fois le réseau revenu.',
  ],
  de: [
    'Keine Verbindung · Sytue Rooms',
    'Keine Verbindung',
    'Sytue Rooms benötigt eine Verbindung zum Laden. Versuche es erneut, sobald du wieder online bist.',
  ],
  it: [
    'Nessuna connessione · Sytue Rooms',
    'Nessuna connessione',
    'Sytue Rooms ha bisogno di una connessione per caricarsi. Riprova quando torni online.',
  ],
};

const OFFLINE_HTML = `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>No connection · Sytue Rooms</title>
<style>html,body{height:100%;margin:0}body{display:grid;place-items:center;background:#faf9f6;color:#22262b;
font:500 15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}main{max-width:22rem;text-align:center;padding:2rem}
h1{font-size:1.1rem;margin:0 0 .5rem}p{margin:0;color:#5b5f66}</style></head>
<body><main><h1>No connection</h1><p>Sytue Rooms needs a connection to load. Try again once you are back online.</p></main>
<script>(function(){var T=${JSON.stringify(OFFLINE_TEXTS)};
var c=/(?:^|;)\\s*sytue_locale=([a-z]{2})/.exec(document.cookie);
var l=c?c[1]:(navigator.language||'en').slice(0,2).toLowerCase();var m=T[l];if(!m)return;
document.documentElement.lang=l;document.title=m[0];
document.querySelector('h1').textContent=m[1];document.querySelector('p').textContent=m[2];})();</script>
</body></html>`;

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches
      .open(STATIC)
      .then((cache) => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(
        keys
          .filter((k) => k.startsWith('sytue-rooms-static-') && k !== STATIC)
          .map((k) => caches.delete(k)),
      );
      await self.clients.claim();
    })(),
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return; // nunca cachear mutaciones (POST/PUT/DELETE)
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return; // solo same-origin

  // Assets inmutables (hash de Astro) + branding: cache-first.
  if (esAsset(url.pathname) || PRECACHE.includes(url.pathname)) {
    event.respondWith(cacheFirst(req));
    return;
  }
  // API/BFF y auth: network-only (datos por-usuario, no cachear). Se deja pasar.
  //
  // Se compara sin el prefijo de la app y en las DOS formas: el shim de `fetch`
  // prefija `/api/…` y `/auth/…`, pero lo que se le escape —un `Request` ya
  // construido, una URL absoluta— llega sin prefijar. Mirando solo el pathname
  // completo, bajo `base` no casaba ninguna: el real es `/rooms/api/…`.
  const rel = url.pathname.startsWith(BASE) ? url.pathname.slice(BASE.length) : url.pathname;
  if (rel.startsWith('/api/') || rel.startsWith('/auth/')) {
    return;
  }
  // Navegaciones (documento HTML): network-first con fallback offline.
  if (req.mode === 'navigate') {
    event.respondWith(networkFirstNavigate(req));
  }
});

async function cacheFirst(req) {
  const cached = await caches.match(req);
  if (cached) return cached;
  const res = await fetch(req);
  if (res.ok) {
    const cache = await caches.open(STATIC);
    cache.put(req, res.clone());
  }
  return res;
}

async function networkFirstNavigate(req) {
  try {
    return await fetch(req);
  } catch {
    const cached = await caches.match(req);
    if (cached) return cached;
    return new Response(OFFLINE_HTML, {
      status: 503,
      headers: { 'content-type': 'text/html; charset=utf-8' },
    });
  }
}

// --- Web Push -----------------------------------------------------------------

self.addEventListener('push', (event) => {
  event.waitUntil(showPush(event));
});

async function showPush(event) {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = {};
  }
  // Dedupe: si la app está ABIERTA en alguna pestaña, la página ya muestra su aviso
  // in-app (mención/DM). El push del SO cubre la app CERRADA — evita duplicar.
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  const appOpen = windows.some((c) => sameOrigin(c.url));
  if (appOpen) return;

  await self.registration.showNotification(data.title || 'Sytue Rooms', {
    body: data.body || '',
    tag: data.tag || 'sytue-rooms',
    icon: `${BASE}/icon-maskable.svg`,
    badge: `${BASE}/icon-maskable.svg`,
    data: { url: data.url || '/' },
  });
}

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || '/';
  event.waitUntil(focusOrOpen(url));
});

async function focusOrOpen(url) {
  // El destino viene en el aviso push. Hoy lo compone el servidor y es una ruta de
  // esta app, pero aquí se abre o se navega una ventana: lo que no caiga en ESTE
  // origen (una URL absoluta de otro sitio, `//otro.sitio`, `javascript:`) no se
  // sigue; se abre la app y nada más.
  let destino;
  try {
    destino = new URL(url, self.location.origin);
  } catch {
    destino = null;
  }
  if (!destino || destino.origin !== self.location.origin) {
    destino = new URL('/', self.location.origin);
  }
  const full = destino.href;
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  for (const c of windows) {
    if (sameOrigin(c.url)) {
      await c.focus();
      // Navega la pestaña al deep-link para que el ChatApp abra el canal/mensaje.
      if ('navigate' in c) {
        try {
          await c.navigate(full);
        } catch {
          /* algunas plataformas no permiten navigate: basta con enfocar */
        }
      }
      return;
    }
  }
  await self.clients.openWindow(full);
}

function sameOrigin(href) {
  try {
    return new URL(href).origin === self.location.origin;
  } catch {
    return false;
  }
}
