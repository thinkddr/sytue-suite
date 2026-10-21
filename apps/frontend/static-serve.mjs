// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Servidor de ficheros estáticos mínimo para `www` (Astro `output: 'static'`).
// Es la única implementación usada por Compose y por la imagen de producción.
import { DEFAULT_LOCALE, negotiateLocale } from '@sytue/i18n';

const ROOT = './dist';
const PORT = Number(process.env.PORT) || 4321;
const HOST = process.env.HOST || '0.0.0.0';

export const SECURITY_HEADERS = {
  'content-security-policy': [
    "default-src 'self'",
    "script-src 'self' 'unsafe-inline'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data:",
    "font-src 'self'",
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'none'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    'upgrade-insecure-requests',
  ].join('; '),
  'strict-transport-security': 'max-age=63072000; includeSubDomains; preload',
  'x-content-type-options': 'nosniff',
  'x-frame-options': 'DENY',
  'referrer-policy': 'strict-origin-when-cross-origin',
  'permissions-policy':
    'accelerometer=(), autoplay=(), camera=(), geolocation=(), gyroscope=(), microphone=(), payment=(), usb=()',
  'cross-origin-opener-policy': 'same-origin',
  'cross-origin-resource-policy': 'same-origin',
};

const LONG_LIVED_EXTENSIONS = new Set([
  'avif',
  'ico',
  'jpeg',
  'jpg',
  'png',
  'svg',
  'webp',
  'woff2',
]);

export function cacheControl(path) {
  if (path.startsWith('_astro/')) return 'public, max-age=31536000, immutable';
  const extension = path.includes('.') ? path.slice(path.lastIndexOf('.') + 1).toLowerCase() : '';
  if (LONG_LIVED_EXTENSIONS.has(extension)) return 'public, max-age=86400';
  return 'public, max-age=300';
}

function response(file, request, status, path) {
  return new Response(request.method === 'HEAD' ? null : file, {
    status,
    headers: {
      ...SECURITY_HEADERS,
      'content-type': file.type || 'application/octet-stream',
      'cache-control': cacheControl(path),
    },
  });
}

/**
 * Rastreadores y previsualizadores: ven `/` tal cual (el inglés, que es el
 * `x-default`) y descubren los demás idiomas por `hreflang`. Redirigirlos por
 * cabecera haría que Google indexara la raíz como alemana si un día rastrea con
 * `Accept-Language: de`.
 */
const BOT = /bot|crawl|spider|slurp|facebookexternalhit|lighthouse/i;

/**
 * Solo la RAÍZ negocia: `/de`, `/fr/downloads`… son URLs explícitas y se sirven
 * siempre. Orden: cookie `sytue_locale` (la que escribe el selector) >
 * `Accept-Language` > inglés, que es la propia `/` y no redirige. La respuesta
 * de `/` depende de esas dos cabeceras, así que va `private` con su `Vary`: una
 * caché compartida no puede guardar la versión de nadie para otro.
 */
const ROOT_HEADERS = {
  vary: 'Accept-Language, Cookie, User-Agent',
  'cache-control': 'private, no-cache',
};

export function rootRedirect(req) {
  if (BOT.test(req.headers.get('user-agent') ?? '')) return null;
  const locale = negotiateLocale(req.headers.get('accept-language'), req.headers.get('cookie'));
  if (locale === DEFAULT_LOCALE) return null;
  return new Response(null, {
    status: 302,
    headers: {
      ...SECURITY_HEADERS,
      location: `/${locale}${new URL(req.url).search}`,
      ...ROOT_HEADERS,
    },
  });
}

export function createStaticHandler(root = ROOT) {
  return async function staticHandler(req) {
    if (req.method !== 'GET' && req.method !== 'HEAD') {
      return new Response(null, {
        status: 405,
        headers: { ...SECURITY_HEADERS, allow: 'GET, HEAD' },
      });
    }
    let path;
    try {
      path = decodeURIComponent(new URL(req.url).pathname).replace(/^\/+|\/+$/g, '');
    } catch {
      return new Response('Bad Request', { status: 400, headers: SECURITY_HEADERS });
    }
    if (path.split('/').includes('..')) {
      return new Response('Bad Request', { status: 400, headers: SECURITY_HEADERS });
    }
    const redirect = path === '' ? rootRedirect(req) : null;
    if (redirect) return redirect;
    const candidates = path === '' ? ['index.html'] : [path, `${path}/index.html`, `${path}.html`];
    for (const candidate of candidates) {
      const file = Bun.file(`${root}/${candidate}`);
      if (await file.exists()) {
        const res = response(file, req, 200, candidate);
        if (path === '') for (const [k, v] of Object.entries(ROOT_HEADERS)) res.headers.set(k, v);
        return res;
      }
    }
    const notFound = Bun.file(`${root}/404.html`);
    if (await notFound.exists()) return response(notFound, req, 404, '404.html');
    return new Response('Not Found', { status: 404, headers: SECURITY_HEADERS });
  };
}

export function startServer({ root = ROOT, port = PORT, hostname = HOST } = {}) {
  const server = Bun.serve({ port, hostname, fetch: createStaticHandler(root) });
  console.log(`[www] static server on http://${hostname}:${port} (root ${root})`);
  return server;
}

if (import.meta.main) startServer();
