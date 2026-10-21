/*
 * GET  /api/projects  -> { live, projects } (lista para la paleta de comandos ⌘K)
 * POST /api/projects
 *   { slug, name, repo_url, framework?, production_ref?, root_dir?, build_* }
 *   -> crea un proyecto
 *
 * Devuelve { ok, live, project? }. live=false en modo demostración.
 *
 * El proyecto de la respuesta incluye `webhookUrl` + `webhookSecret` cuando el
 * control-plane los entrega, que es SOLO al crear. Van al navegador a propósito:
 * el secreto existe para que un humano lo pegue en su proveedor git y no hay
 * segunda oportunidad de leerlo. Se devuelve en un POST sin caché (`no-store`) y
 * nunca se persiste ni se vuelve a servir desde el listado.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { createProject, loadProjects, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;
/** Nombre de rama/etiqueta git: sin espacios, sin `..`, sin caracteres de control. */
const REF_RE = /^(?!.*\.\.)[A-Za-z0-9][A-Za-z0-9._\-/]{0,254}$/;
/** Subdirectorio RELATIVO del repo: sin `..`, sin raíz absoluta, sin `~`. */
const ROOT_DIR_RE = /^(?!.*\.\.)[A-Za-z0-9._-]+(\/[A-Za-z0-9._-]+)*$/;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const { live, data } = await loadProjects(session);
  return json({ live, projects: data });
};

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    slug?: unknown;
    name?: unknown;
    repo_url?: unknown;
    framework?: unknown;
    build_strategy?: unknown;
    build_command?: unknown;
    install_command?: unknown;
    output_dir?: unknown;
    production_ref?: unknown;
    root_dir?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const slug = typeof body.slug === 'string' ? body.slug.trim().toLowerCase() : '';
  const name = typeof body.name === 'string' ? body.name.trim() : slug;
  const repoUrl = typeof body.repo_url === 'string' ? body.repo_url.trim() : '';
  const str = (v: unknown): string => (typeof v === 'string' ? v.trim() : '');
  const framework = str(body.framework);
  const buildStrategy = str(body.build_strategy);
  const buildCommand = str(body.build_command);
  const installCommand = str(body.install_command);
  const outputDir = str(body.output_dir);
  const productionRef = str(body.production_ref);
  const rootDir = str(body.root_dir).replace(/^\/+|\/+$/g, '');

  if (!SLUG_RE.test(slug)) return json({ error: 'invalid_slug' }, 400);
  if (!repoUrl.startsWith('https://')) return json({ error: 'invalid_repo_url' }, 400);
  if (productionRef && !REF_RE.test(productionRef)) return json({ error: 'invalid_ref' }, 400);
  // `..` fuera: el root_dir se concatena a la copia del repo en el builder, y un
  // salto de directorio ahí es la vía para construir algo que no es del usuario.
  if (rootDir && !ROOT_DIR_RE.test(rootDir)) return json({ error: 'invalid_root_dir' }, 400);

  try {
    const result = await createProject(session, {
      slug,
      name,
      repoUrl,
      ...(framework ? { framework } : {}),
      ...(buildStrategy ? { buildStrategy } : {}),
      ...(buildCommand ? { buildCommand } : {}),
      ...(installCommand ? { installCommand } : {}),
      ...(outputDir ? { outputDir } : {}),
      ...(productionRef ? { productionRef } : {}),
      ...(rootDir ? { rootDir } : {}),
    });
    return json({ ok: true, live: result.live, project: result.data });
  } catch (err) {
    // Error REAL del control-plane (p. ej. slug en conflicto/quota): status real a la UI.
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
