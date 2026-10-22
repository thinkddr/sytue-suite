/*
 * POST /api/env
 *   { project, key, value, secret?, scope? } -> crea/actualiza (PUT en el control-plane)
 *   { project, key, delete: true }           -> elimina
 *
 * `scope` (all|production|preview) es lo que permite que una rama sin revisar
 * corra con una credencial distinta de la de producción; se omitía, así que TODA
 * variable acababa en los dos entornos.
 *
 * BFF de variables y secretos del proyecto. El borrado tambien viaja por POST
 * porque el checkOrigin CSRF de Astro rechaza DELETE con 403 (mismo patron que
 * Mail /api/drafts). Devuelve { ok, live }: live=false cuando no hay backend
 * vivo (dev/local) y el cambio queda en demostracion. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import type { EnvScope } from '@sytue/api-clients';
import { bff, toBffContext } from '../../lib/auth.js';
import { saveEnv, deleteEnv, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

/** Clave de variable de entorno valida (letras/digitos/_, sin empezar por digito). */
const KEY_RE = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/;
/** Ambitos que acepta el control-plane (domain.EnvScope). */
const SCOPES: EnvScope[] = ['all', 'production', 'preview'];

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    project?: unknown;
    key?: unknown;
    value?: unknown;
    secret?: unknown;
    scope?: unknown;
    delete?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const project = typeof body.project === 'string' ? body.project.trim() : '';
  const key = typeof body.key === 'string' ? body.key.trim() : '';
  if (!project) return json({ error: 'missing_project' }, 400);
  if (!KEY_RE.test(key)) return json({ error: 'invalid_key' }, 400);

  try {
    if (body.delete === true) {
      const live = await deleteEnv(session, project, key);
      return json({ ok: true, live });
    }

    const value = typeof body.value === 'string' ? body.value : '';
    if (!value.trim()) return json({ error: 'empty_value' }, 400);

    // Un ámbito que la API no conoce se rechaza aquí: dejarlo pasar haría que el
    // control-plane cayera a su defecto («los dos») justo cuando el usuario pidió
    // acotarlo, que es lo contrario de lo que creería haber guardado.
    const scope = typeof body.scope === 'string' ? body.scope : '';
    if (scope && !SCOPES.includes(scope as EnvScope)) return json({ error: 'invalid_scope' }, 400);

    const live = await saveEnv(session, project, {
      key,
      value,
      secret: body.secret === true,
      ...(scope ? { scope: scope as EnvScope } : {}),
    });
    return json({ ok: true, live });
  } catch (err) {
    // Error REAL del control-plane (quota_exceeded/forbidden/...): status real a la UI.
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
