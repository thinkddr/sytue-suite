/*
 * POST /api/inspect-repo -> { ok, live, detected, reason?, detection? }
 *
 * ¿Qué hay en el repositorio que acabo de pegar? El control-plane lo mira por la
 * API de su forja y devuelve framework, comandos y directorio de salida, para que
 * el formulario de alta no tenga que preguntarlos: están escritos en el repo, y
 * `output_dir` no es una pregunta para quien empieza — es un muro.
 *
 * Va por POST y no por GET por la misma razón que /api/check-slug: la respuesta
 * depende del contenido del repositorio y cambia con cada push, así que como GET
 * sería cacheable y el formulario acabaría rellenándose con lo que el proyecto era
 * hace dos semanas.
 *
 * `live:false` (sin control-plane) y `detected:false` (lo hay, pero no se pudo
 * mirar) acaban igual en la interfaz —se enseñan los campos— pero se distinguen
 * aquí: solo el segundo trae un motivo que decirle al usuario. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { inspectRepo, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as {
    repo_url?: string;
    root_dir?: string;
  };
  const repoUrl = (body.repo_url ?? '').trim();
  if (!repoUrl) return json({ error: 'repo_url_required' }, 400);
  try {
    const result = await inspectRepo(session, repoUrl, (body.root_dir ?? '').trim());
    if (!result) return json({ ok: true, live: false, detected: false });
    return json({ ok: true, live: true, ...result });
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
