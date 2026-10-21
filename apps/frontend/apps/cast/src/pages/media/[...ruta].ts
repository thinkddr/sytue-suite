/**
 * /cast/media/<video>/<token>/<fichero> — los ficheros de un vídeo (HLS, WebM,
 * póster, original) servidos por el servicio en `/media/…` con su token firmado.
 *
 * SIN SESIÓN, y a propósito: la credencial es el token que emitió `play` tras
 * pasar la ACL, y lo pide también el reproductor incrustado en un sitio ajeno,
 * que no tiene cookie. Por eso el enlace va bajo el prefijo de la app y no a la
 * raíz del host: la página vive en `<org>.<dominio>/cast`, donde `/media` no
 * existe, y así funciona igual en los dos hosts.
 *
 * Se reenvía `Range` y se devuelve `206` con su `Content-Range`: sin eso el
 * reproductor no puede buscar en el WebM ni pedir un segmento a medias.
 */
import type { APIRoute } from 'astro';
import { rutaQueEscapa } from '@sytue/auth/server';
import { DESTINO } from '../../lib/servicio.js';

export const prerender = false;

const HACIA_FUERA = [
  'content-type',
  'content-length',
  'content-range',
  'accept-ranges',
  'cache-control',
  'last-modified',
];

const media: APIRoute = async (ctx) => {
  const ruta = ctx.params.ruta ?? '';
  if (rutaQueEscapa(ruta)) return new Response(null, { status: 400 });
  const rango = ctx.request.headers.get('range');
  let res: Response;
  try {
    res = await fetch(`${DESTINO}/media/${ruta}`, {
      method: ctx.request.method,
      headers: rango ? { range: rango } : {},
    });
  } catch {
    return new Response(null, { status: 503 });
  }
  const salida = new Headers();
  for (const n of HACIA_FUERA) {
    const v = res.headers.get(n);
    if (v) salida.set(n, v);
  }
  // El incrustado vive en otro origen y pide los segmentos por XHR (hls.js): el
  // fichero es público para quien tenga el token, así que se deja leer.
  salida.set('cross-origin-resource-policy', 'cross-origin');
  return new Response(res.body, { status: res.status, headers: salida });
};

export const GET = media;
export const HEAD = media;
