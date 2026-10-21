/*
 * POST /api/storage
 *   multipart { project, file }        -> sube el fichero al bucket del proyecto
 *   json { project, key, delete:true } -> borra ese objeto
 *   json { project, key, download:true } -> devuelve { url } prefirmada (15 min)
 *
 * BFF de Sytue Storage. El panel listaba el bucket y enseñaba las credenciales, y
 * ahí se acababa: no había forma de subir, descargar ni borrar nada desde la web
 * aunque el control-plane firma URLs desde la Ola E+F. Todo va por POST porque el
 * checkOrigin CSRF de Astro rechaza DELETE (mismo patrón que /api/env).
 *
 * La subida pasa por este proceso a propósito (ver uploadStorageObject): el PUT
 * directo desde el navegador necesitaría CORS en el bucket, que los ya
 * aprovisionados no tienen. Tope de 25 MiB para no cargar en memoria lo que le
 * echen. NUNCA expone tokens ni la clave secreta del bucket.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import {
  MAX_UPLOAD_BYTES,
  deleteStorageObject,
  httpErrorInfo,
  presignDownload,
  uploadStorageObject,
} from '../../lib/cloud.js';

export const prerender = false;

/**
 * Clave admisible: sin `..`, sin barra inicial y sin caracteres de control. El
 * aislamiento REAL lo da el control-plane —resuelve el bucket desde el proyecto
 * bajo RLS, así que la clave nunca elige bucket—; esto solo evita nombres que
 * confunden a quien los lee después en la lista.
 */
function validKey(key: string): boolean {
  if (!key || key.length > 1024) return false;
  if (key.startsWith('/') || key.includes('..')) return false;
  for (const ch of key) {
    const c = ch.charCodeAt(0);
    if (c < 0x20 || c === 0x7f) return false;
  }
  return true;
}

/**
 * Campo de texto del formulario. `FormData.get` devuelve `File | string | null`,
 * y un `String(...)` sobre un File daría «[object File]» como si fuera un valor
 * legítimo: aquí lo que no sea texto no existe.
 */
function formText(form: FormData, key: string): string {
  const v = form.get(key);
  return typeof v === 'string' ? v.trim() : '';
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const contentType = ctx.request.headers.get('content-type') ?? '';

  try {
    if (contentType.includes('multipart/form-data')) {
      const form = await ctx.request.formData();
      const project = formText(form, 'project');
      const file = form.get('file');
      if (!project) return json({ error: 'missing_project' }, 400);
      if (!(file instanceof File) || file.size === 0) return json({ error: 'missing_file' }, 400);
      if (file.size > MAX_UPLOAD_BYTES) return json({ error: 'file_too_large' }, 413);

      // El prefijo lo teclea quien sube; el nombre sale del propio fichero, que
      // es lo que espera cualquiera que después mire la lista.
      const prefix = formText(form, 'prefix').replace(/^\/+|\/+$/g, '');
      const key = prefix ? `${prefix}/${file.name}` : file.name;
      if (!validKey(key)) return json({ error: 'invalid_key' }, 400);

      const live = await uploadStorageObject(
        session,
        project,
        key,
        await file.arrayBuffer(),
        file.type,
      );
      return json({ ok: true, live, key });
    }

    const body = (await ctx.request.json()) as {
      project?: unknown;
      key?: unknown;
      delete?: unknown;
      download?: unknown;
    };
    const project = typeof body.project === 'string' ? body.project.trim() : '';
    const key = typeof body.key === 'string' ? body.key.trim() : '';
    if (!project) return json({ error: 'missing_project' }, 400);
    if (!key) return json({ error: 'missing_key' }, 400);

    if (body.delete === true) {
      const live = await deleteStorageObject(session, project, key);
      return json({ ok: true, live });
    }
    if (body.download === true) {
      const url = await presignDownload(session, project, key);
      return json({ ok: true, live: Boolean(url), ...(url ? { url } : {}) });
    }
    return json({ error: 'no_action' }, 400);
  } catch (err) {
    // Error REAL del control-plane (403 sin rol, 404 sin bucket…): status real.
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    throw err;
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
