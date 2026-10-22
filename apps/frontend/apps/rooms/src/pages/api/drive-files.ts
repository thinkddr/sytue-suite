/**
 * GET /api/drive-files?path=/ruta[&cursor=<opaco>] — lista una carpeta de Sytue
 * Drive para el selector «Archivo de Sytue Drive» del menú «+» del compositor.
 * Mismo patrón que el endpoint homónimo de Sytue Mail.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly: usa el access token de la
 * sesión contra la API REST de Sytue Drive (DriveClient de @sytue/api-clients). El
 * token NUNCA llega al navegador. La AUTORIZACIÓN la aplica Drive: solo devuelve lo
 * que este usuario puede ver (no se puentea el permiso). Sin lógica de negocio aquí.
 *
 * Paginación KEYSET: las subcarpetas van completas en la primera página; los
 * ficheros llegan por páginas (`nextCursor`), sin traer toda la unidad de golpe.
 *
 * A diferencia de Mail (que adjunta un ENLACE público), en Rooms un adjunto YA ES una
 * referencia a un fichero de Drive por `file_id`: se devuelve `id` + `contentType` +
 * `size` y el compositor los adjunta tal cual, sin re-subir bytes.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer, HttpError } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { driveBaseUrl } from '../../lib/drive.js';

export const prerender = false;

/** Ítem del selector: carpeta (navegable) o fichero (adjuntable por `id`). */
interface PickerItem {
  id?: string | undefined;
  name: string;
  isDirectory: boolean;
  path: string;
  size?: number | undefined;
  contentType?: string | undefined;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = new URL(ctx.request.url).searchParams;
  const path = params.get('path') || '/';
  const cursor = params.get('cursor')?.trim() || '';

  const client = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.accessToken),
  });

  try {
    const page = await client.listPage(path, { ...(cursor ? { cursor } : {}), limit: 100 });
    const items: PickerItem[] = page.items.map((r) => ({
      ...(r.id ? { id: r.id } : {}),
      name: r.name,
      isDirectory: r.isDirectory,
      path: r.path,
      ...(r.size !== undefined ? { size: r.size } : {}),
      ...(r.contentType ? { contentType: r.contentType } : {}),
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
