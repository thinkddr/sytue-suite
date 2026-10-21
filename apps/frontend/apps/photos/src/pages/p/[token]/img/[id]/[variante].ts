/**
 * Imagen de un álbum compartido, SIN sesión.
 *
 * El navegador de quien abre el enlace no tiene cookie ni token, así que esto
 * no puede pasar por el proxy del BFF. Va contra `/public/albums/{token}/…` del
 * servicio, que comprueba que la foto esté en ESE álbum antes de servirla: el
 * token de la ruta es toda la credencial, y sin esa comprobación sería una
 * licencia para pedir cualquier identificador de la organización.
 *
 * El servicio queda dentro: aquí se reenvía por la red interna, en streaming y
 * conservando las cabeceras de caché, que es lo que hace barata la cuadrícula.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';

export const prerender = false;

const env = serverEnv(import.meta.env);
const destino = (env.SYTUE_PHOTOS_API_URL ?? 'http://syphotos:8097').replace(/\/$/, '');

export const GET: APIRoute = async ({ params }) => {
  const { token, id, variante } = params;
  if (!token || !id || !variante) return new Response(null, { status: 404 });

  const res = await fetch(
    `${destino}/public/albums/${encodeURIComponent(token)}/assets/${encodeURIComponent(id)}` +
      `/variants/${encodeURIComponent(variante)}`,
    { headers: { accept: 'image/jpeg' } },
  );
  if (!res.ok) return new Response(null, { status: res.status === 404 ? 404 : 502 });

  const cabeceras = new Headers();
  for (const n of ['content-type', 'content-length', 'cache-control']) {
    const v = res.headers.get(n);
    if (v) cabeceras.set(n, v);
  }
  cabeceras.set('x-content-type-options', 'nosniff');
  cabeceras.set('x-robots-tag', 'noindex, nofollow');
  return new Response(res.body, { status: 200, headers: cabeceras });
};
