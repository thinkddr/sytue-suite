/**
 * GET /api/drive-files?path=/ruta[&cursor=<opaco>] — lista una carpeta de Sytue
 * Drive para el selector «Adjuntar desde Drive» del compositor (intent cross-app
 * attach-drive-to-mail, docs/CONVENTIONS.md §11).
 *
 * GET /api/drive-files?id=<uuid>[&id=…] — resuelve ficheros CONCRETOS por su id.
 * Lo usa el otro extremo del mismo intent: cuando alguien llega desde Drive con
 * «Adjuntar a un correo», el deep-link solo trae ids —los nombres no viajan en la
 * URL, que es de datos de usuario y acabaría siendo larga y fea—, y el compositor
 * necesita el nombre para pintar el adjunto. Se resuelve AQUÍ y no en el
 * navegador porque el token de la sesión no sale del servidor, y así el permiso
 * lo sigue aplicando Drive fichero a fichero.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly: usa el access token de la
 * sesión de Mail contra la API REST de Sytue Drive (vía DriveClient de
 * @sytue/api-clients). El token NUNCA llega al navegador. La AUTORIZACIÓN la aplica
 * Drive: solo devuelve lo que este usuario puede ver (no se puentea el permiso).
 *
 * Paginación KEYSET: las subcarpetas van completas en la primera página; los
 * ficheros llegan por páginas (`nextCursor`), sin traer toda la unidad de golpe.
 * Devuelve por fichero un `link` público ya compuesto ({drive}/files/{id}) para
 * adjuntarlo como referencia sin exponer la URL pública de Drive al cliente.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer, HttpError } from '@sytue/api-clients';
import { driveFileUrl } from '@sytue/config';
import { getApiSession } from '../../lib/session.js';
import { serverEnv } from '@sytue/auth/server';

export const prerender = false;

const env = serverEnv(import.meta.env);

/** URL interna de la API de Sytue Drive (server-side). */
function driveApiUrl(): string {
  // El Service del clúster primero: desde un pod, el nombre público no resuelve
  // y la llamada ni llega a ser HTTP (sale como 502).
  return (
    env.SYTUE_DRIVE_INTERNAL_URL ??
    env.SYTUE_DRIVE_API_URL ??
    env.PUBLIC_SYTUE_DRIVE_BASE_URL ??
    'https://drive.sytue.eu'
  );
}

/** Ítem del selector: carpeta (navegable) o fichero (adjuntable por `link`). */
interface PickerItem {
  id?: string | undefined;
  name: string;
  isDirectory: boolean;
  path: string;
  size?: number | undefined;
  /** Enlace público al recurso (solo ficheros); referencia que se adjunta al correo. */
  link?: string | undefined;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = new URL(ctx.request.url).searchParams;
  const path = params.get('path') || '/';
  const cursor = params.get('cursor')?.trim() || '';

  const client = new DriveClient({
    baseUrl: driveApiUrl(),
    auth: bearer(session.accessToken),
  });

  // Resolución por id. Se acota a 20: es el tope de ficheros que tiene sentido
  // adjuntar de una vez, y sin tope una URL fabricada a mano podría encargar
  // cientos de consultas a Drive con una sola petición.
  const ids = params
    .getAll('id')
    .map((s) => s.trim())
    .filter(Boolean)
    .slice(0, 20);
  if (ids.length > 0) {
    // En paralelo y tolerante: un id que ya no existe —o que este usuario no
    // puede ver— no puede tumbar el resto. Se omite, y el compositor abre con los
    // que sí valen en vez de con un error que no dice cuál falló.
    const resueltos = await Promise.all(
      ids.map(async (id) => {
        try {
          const r = await client.statById(id);
          return { id, name: r.name, link: driveFileUrl(id) };
        } catch {
          return null;
        }
      }),
    );
    return json({ items: resueltos.filter((x) => x !== null) });
  }

  try {
    const page = await client.listPage(path, { ...(cursor ? { cursor } : {}), limit: 100 });
    const items: PickerItem[] = page.items.map((r) => ({
      ...(r.id ? { id: r.id } : {}),
      name: r.name,
      isDirectory: r.isDirectory,
      path: r.path,
      ...(r.size !== undefined ? { size: r.size } : {}),
      ...(!r.isDirectory && r.id ? { link: driveFileUrl(r.id) } : {}),
    }));
    return json({ items, ...(page.nextCursor ? { nextCursor: page.nextCursor } : {}) });
  } catch (e) {
    // 4xx de Drive (p. ej. 403 sin permiso, 404 carpeta inexistente) se preservan;
    // el resto se colapsa a 502. El selector muestra su estado de error/vacío.
    if (e instanceof HttpError && e.status >= 400 && e.status < 500) {
      return json({ error: 'drive_error' }, e.status);
    }
    return json({ error: 'drive_unreachable' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
