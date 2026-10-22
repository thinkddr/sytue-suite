/*
 * POST /api/projects/from-template
 *   { template_id, slug, name?, repo_url, answers? }
 *   -> da de alta el proyecto ENTERO desde una plantilla del catálogo
 *
 * Proxy PURO al control-plane (POST /api/v1/projects:from-template); sin lógica
 * (invariante del repo). Ahí dentro, y en UNA transacción, se crean el proyecto,
 * los servicios gestionados que declara la plantilla, sus variables de entorno
 * (con los secretos GENERADOS y cifrados por el servidor) y el primer despliegue.
 *
 * Devuelve { ok, live, result? }. `live:false` = no hay control-plane conectado y
 * NO se ha creado nada: no se fabrica un proyecto de mentira. Los errores reales
 * (409 slug cogido, 402 cuota/gasto/suscripción, 403 rol, 400 respuesta que la
 * plantilla no declara) se propagan con SU status y SU código.
 *
 * `result.webhook_secret` viaja al navegador a propósito: el control-plane solo lo
 * entrega aquí y una vez, y existe para que un humano lo pegue en su proveedor
 * git. Respuesta sin caché (`no-store`); nunca se persiste ni se vuelve a servir.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { createProjectFromTemplate, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    template_id?: unknown;
    slug?: unknown;
    name?: unknown;
    repo_url?: unknown;
    answers?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const str = (v: unknown): string => (typeof v === 'string' ? v.trim() : '');
  const templateId = str(body.template_id);
  const slug = str(body.slug).toLowerCase();
  const repoUrl = str(body.repo_url);
  const name = str(body.name) || slug;
  if (!templateId) return json({ error: 'missing_template' }, 400);
  if (!slug) return json({ error: 'missing_slug' }, 400);
  if (!repoUrl) return json({ error: 'missing_repo_url' }, 400);

  // Las respuestas se pasan tal cual: quién puede responder qué lo decide la
  // plantilla, y eso lo valida el control-plane (una clave que no declara es un
  // 400). Aquí solo se garantiza que es un mapa de cadenas, no JSON arbitrario.
  const answers: Record<string, string> = {};
  if (body.answers !== null && typeof body.answers === 'object') {
    for (const [k, v] of Object.entries(body.answers as Record<string, unknown>)) {
      if (typeof v === 'string') answers[k] = v;
    }
  }

  try {
    const { live, result } = await createProjectFromTemplate(session, {
      templateId,
      slug,
      name,
      repoUrl,
      answers,
    });
    return json({ ok: true, live, ...(result ? { result } : {}) }, live ? 201 : 200);
  } catch (err) {
    // Status y código REALES del control-plane; la interfaz los traduce por código.
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
