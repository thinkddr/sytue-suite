/**
 * GET /api/shared[?album=<id>] — «Compartido conmigo»: los álbumes que otras personas
 * han compartido con quien pregunta o, con `album`, uno de ellos con sus fotos.
 *
 * Por el BFF y no por el proxy genérico por UNA cosa: el servicio dice quién lo
 * compartió por su `sub`, y aquí se le pone nombre con el directorio de la
 * organización. Las imágenes sí van por el proxy (`/api/v1/shared/albums/…`).
 */
import type { APIRoute } from 'astro';
import { crearDirectorio, serverEnv } from '@sytue/auth/server';
import { bff, getApiSession } from '../../lib/bff.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const destino = (env.SYTUE_PHOTOS_API_URL ?? 'http://syphotos:8097').replace(/\/$/, '');
const { resolverPersonas } = crearDirectorio(bff, import.meta.env);

interface AlbumCompartido {
  sharedBy: string;
  sharedByName?: string;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const sesion = await getApiSession(ctx);
  if (!sesion) return json({ error: 'unauthenticated' }, 401);
  const album = new URL(ctx.request.url).searchParams.get('album');
  const ruta = album ? `/shared/albums/${encodeURIComponent(album)}` : '/shared/albums';
  let res: Response;
  try {
    res = await fetch(`${destino}/api/v1${ruta}`, {
      headers: { authorization: `Bearer ${sesion.accessToken}`, accept: 'application/json' },
    });
  } catch {
    return json({ error: 'upstream_unreachable' }, 502);
  }
  if (!res.ok) return json({ error: 'upstream' }, res.status);
  const datos = (await res.json()) as { items?: AlbumCompartido[]; album?: AlbumCompartido };
  const albumes = datos.album ? [datos.album] : (datos.items ?? []);
  const caras = await resolverPersonas(
    ctx,
    albumes.map((a) => a.sharedBy),
  );
  for (const a of albumes) {
    const c = caras.get(a.sharedBy);
    const nombre = c?.name ?? c?.email;
    if (nombre) a.sharedByName = nombre;
  }
  return json(datos);
};
