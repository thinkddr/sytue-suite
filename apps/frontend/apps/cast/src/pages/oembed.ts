/**
 * GET /cast/oembed?url=…&maxwidth=…&maxheight=…&format=json — oEmbed 1.0 de un
 * vídeo público o no listado. Es el servicio quien lo compone (`/public/oembed`, con
 * el `<iframe>` de /cast/embed/<id>); aquí solo se reenvía la consulta, para que el
 * descubrimiento funcione en el host de la web (`<link rel="alternate">` de /watch).
 */
import type { APIRoute } from 'astro';
import { DESTINO } from '../lib/servicio.js';

export const prerender = false;

const PERMITIDOS = ['url', 'maxwidth', 'maxheight', 'format'] as const;

export const GET: APIRoute = async ({ url }) => {
  const q = new URLSearchParams();
  for (const k of PERMITIDOS) {
    const v = url.searchParams.get(k);
    if (v) q.set(k, v);
  }
  try {
    const res = await fetch(`${DESTINO}/api/v1/public/oembed?${q.toString()}`, {
      headers: { accept: 'application/json' },
    });
    return new Response(res.body, {
      status: res.status,
      headers: {
        'content-type': res.headers.get('content-type') ?? 'application/json',
        'cache-control': res.ok ? 'public, max-age=300' : 'no-store',
        'access-control-allow-origin': '*',
      },
    });
  } catch {
    return new Response(JSON.stringify({ error: { code: 'unavailable', message: '' } }), {
      status: 503,
      headers: { 'content-type': 'application/json' },
    });
  }
};
