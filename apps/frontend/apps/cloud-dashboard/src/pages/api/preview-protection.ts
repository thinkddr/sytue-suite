/*
 * POST /api/preview-protection { project }                  -> cierra (o rota)
 * POST /api/preview-protection { project, disable: true }   -> vuelve a abrir
 *
 * BFF de la protección de previews. Proxy PURO al control-plane
 * (POST/DELETE /api/v1/projects/{id}/preview-protection); sin lógica
 * (invariante). El «abrir» viaja por POST porque el checkOrigin CSRF de Astro
 * rechaza DELETE con 403 (mismo patrón que /api/env).
 *
 * La contraseña vuelve EN CLARO en el cuerpo, con `no-store`: es su única razón
 * de existir y no se puede volver a leer —lo que se guarda es su bcrypt—. La
 * autorización (admin/owner, solo humanos) la impone el control-plane. NUNCA
 * expone tokens de sesión.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { protectPreviews, unprotectPreviews, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { project?: unknown; disable?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const project = typeof body.project === 'string' ? body.project.trim() : '';
  if (!project) return json({ error: 'missing_project' }, 400);

  try {
    if (body.disable === true) {
      const live = await unprotectPreviews(session, project);
      return json({ ok: true, live, protected: false });
    }
    const creds = await protectPreviews(session, project);
    return json({
      ok: true,
      live: Boolean(creds),
      protected: Boolean(creds),
      ...(creds ? { username: creds.username, password: creds.password } : {}),
    });
  } catch (err) {
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
