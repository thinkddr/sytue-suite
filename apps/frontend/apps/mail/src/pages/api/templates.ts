/**
 * /api/templates — plantillas de respuesta de la cuenta, proxy BFF al servicio Go
 * (REST /api/v1/templates…). GET lista; POST multiplexa por la forma del cuerpo,
 * como /api/labels, porque el checkOrigin CSRF de Astro rechaza PUT y DELETE desde
 * el navegador:
 *   { name, body }              → crear
 *   { id, name, body }          → cambiar nombre y texto
 *   { id, delete: true }        → borrar
 *
 * Los rechazos que la pantalla sabe explicar (nombre repetido, tope de plantillas,
 * texto demasiado largo, ya no existe) conservan su estado y el `code` estable del
 * servicio; el resto es un 502. Sin eso, «ya tienes una que se llama así» y «no hay
 * red» acababan en el mismo aviso genérico.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost, mailPut, mailDelete } from '../../lib/mailrest.js';
import { json, rechazoDelServicio as rechazo } from '../../lib/bffRespuestas.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;

interface RawTemplate {
  id: string;
  name?: string;
  body?: string;
  updated_at?: string;
}

/** La plantilla del Go (snake_case) en la forma camelCase del cliente. */
function mapTemplate(t: RawTemplate) {
  return { id: t.id, name: t.name ?? '', body: t.body ?? '', updatedAt: t.updated_at ?? '' };
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/templates');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { templates?: RawTemplate[] };
    return json({ templates: (body.templates ?? []).map(mapTemplate) });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

interface Payload {
  id?: string;
  name?: string;
  body?: string;
  delete?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  const token = session.accessToken;

  try {
    if (body.id !== undefined && !UUID.test(body.id)) return json({ error: 'invalid_id' }, 400);
    // Borrar.
    if (body.id && body.delete === true) {
      const res = await mailDelete(token, `/api/v1/templates/${body.id}`);
      if (!res.ok) return rechazo(res);
      return json({ ok: true });
    }
    if (typeof body.name !== 'string') return json({ error: 'invalid_payload' }, 400);
    const datos = { name: body.name, body: typeof body.body === 'string' ? body.body : '' };
    // Cambiar.
    if (body.id) {
      const res = await mailPut(token, `/api/v1/templates/${body.id}`, datos);
      if (!res.ok) return rechazo(res);
      return json({ template: mapTemplate((await res.json()) as RawTemplate) });
    }
    // Crear.
    const res = await mailPost(token, '/api/v1/templates', datos);
    if (!res.ok) return rechazo(res);
    return json({ template: mapTemplate((await res.json()) as RawTemplate) }, 201);
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};
