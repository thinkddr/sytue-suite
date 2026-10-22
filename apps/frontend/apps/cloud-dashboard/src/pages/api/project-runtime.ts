/*
 * POST /api/project-runtime
 *   { project, instance_size?, instance_count?, app_port?, health_path?,
 *     repo_url?, production_ref?, release_command? }
 *
 * Cambia la ESCALA y el contrato de ejecución de una app. Va por POST y no por
 * PATCH porque el checkOrigin CSRF de Astro rechaza PATCH con 403 (mismo patrón
 * que /api/env y /api/uptime).
 *
 * Solo viajan al control-plane las claves PRESENTES en el cuerpo: la API trata lo
 * ausente como «no lo toques», así que enviar el objeto entero machacaría la
 * configuración que no se está editando.
 *
 * La validación de aquí es de FORMA (tipo y rango), para dar un 400 explicativo en
 * vez de un error opaco; la autoridad sigue siendo el control-plane, que valida el
 * tamaño contra su catálogo y aplica la cuota del plan. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { updateProjectRuntime, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

/** Tope duro de instancias del dominio (domain.MaxInstanceCount). */
const MAX_INSTANCES = 50;
const MAX_HEALTH_PATH = 255;
/** Nombre de rama/etiqueta git: sin espacios, sin `..`, sin caracteres de control. */
const REF_RE = /^(?!.*\.\.)[A-Za-z0-9][A-Za-z0-9._\-/]{0,254}$/;

function isInt(v: unknown, min: number, max: number): v is number {
  return typeof v === 'number' && Number.isInteger(v) && v >= min && v <= max;
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    project?: unknown;
    instance_size?: unknown;
    instance_count?: unknown;
    app_port?: unknown;
    health_path?: unknown;
    repo_url?: unknown;
    production_ref?: unknown;
    release_command?: unknown;
    paused?: unknown;
    http_rules?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const project = typeof body.project === 'string' ? body.project.trim() : '';
  if (!project) return json({ error: 'missing_project' }, 400);

  const patch: {
    instance_size?: string;
    instance_count?: number;
    app_port?: number;
    health_path?: string;
    repo_url?: string;
    production_ref?: string;
    release_command?: string;
    paused?: boolean;
    http_rules?: {
      redirects?: Array<{ from: string; to: string; status?: number }>;
      headers?: Record<string, string>;
    };
  } = {};

  if (body.instance_size !== undefined) {
    // El nombre se valida contra el catálogo en el control-plane; aquí solo se
    // comprueba que sea una cadena con forma de identificador.
    const size = typeof body.instance_size === 'string' ? body.instance_size.trim() : '';
    if (!/^[a-z0-9-]{1,32}$/.test(size)) return json({ error: 'invalid_instance_size' }, 400);
    patch.instance_size = size;
  }
  if (body.instance_count !== undefined) {
    if (!isInt(body.instance_count, 1, MAX_INSTANCES)) {
      return json({ error: 'invalid_instance_count' }, 400);
    }
    patch.instance_count = body.instance_count;
  }
  if (body.app_port !== undefined) {
    if (!isInt(body.app_port, 1, 65535)) return json({ error: 'invalid_app_port' }, 400);
    patch.app_port = body.app_port;
  }
  if (body.health_path !== undefined) {
    const path = typeof body.health_path === 'string' ? body.health_path.trim() : '';
    if (!path.startsWith('/') || path.length > MAX_HEALTH_PATH) {
      return json({ error: 'invalid_health_path' }, 400);
    }
    patch.health_path = path;
  }

  if (body.repo_url !== undefined) {
    // https obligatorio, igual que en el alta: el clon lo hace el builder, y un
    // esquema git:// o ssh:// ahí no lleva credencial ni verifica al servidor.
    const repo = typeof body.repo_url === 'string' ? body.repo_url.trim() : '';
    if (!repo.startsWith('https://') || repo.length > 512) {
      return json({ error: 'invalid_repo_url' }, 400);
    }
    patch.repo_url = repo;
  }
  if (body.production_ref !== undefined) {
    const ref = typeof body.production_ref === 'string' ? body.production_ref.trim() : '';
    if (!REF_RE.test(ref)) return json({ error: 'invalid_ref' }, 400);
    patch.production_ref = ref;
  }

  if (body.release_command !== undefined) {
    // Cadena vacía es un valor VÁLIDO aquí: significa «quítalo». Por eso no se
    // descarta como en los demás campos, donde vacío es «no lo toques».
    const cmd = typeof body.release_command === 'string' ? body.release_command.trim() : '';
    // El tope lo repite el control-plane (domain.MaxReleaseCommandLen) y el CHECK
    // de la migración; aquí solo evita mandar algo que va a rebotar.
    if (cmd.length > 2000) return json({ error: 'invalid_release_command' }, 400);
    patch.release_command = cmd;
  }

  if (body.paused !== undefined) {
    if (typeof body.paused !== 'boolean') return json({ error: 'invalid_paused' }, 400);
    patch.paused = body.paused;
  }

  if (body.http_rules !== undefined) {
    // Forma nada más: qué es una redirección válida, qué cabeceras están
    // prohibidas y que ningún valor lleve saltos de línea lo decide el
    // control-plane, que es donde vive el criterio (y donde está el riesgo).
    const raw = body.http_rules;
    if (typeof raw !== 'object' || raw === null) return json({ error: 'invalid_http_rules' }, 400);
    const src = raw as { redirects?: unknown; headers?: unknown };
    const rules: {
      redirects?: Array<{ from: string; to: string; status?: number }>;
      headers?: Record<string, string>;
    } = {};
    if (Array.isArray(src.redirects)) {
      rules.redirects = src.redirects.slice(0, 20).flatMap((r) => {
        if (typeof r !== 'object' || r === null) return [];
        const o = r as { from?: unknown; to?: unknown; status?: unknown };
        if (typeof o.from !== 'string' || typeof o.to !== 'string') return [];
        return [
          {
            from: o.from.trim().slice(0, 512),
            to: o.to.trim().slice(0, 1024),
            ...(typeof o.status === 'number' ? { status: o.status } : {}),
          },
        ];
      });
    }
    if (typeof src.headers === 'object' && src.headers !== null) {
      const out: Record<string, string> = {};
      for (const [k, v] of Object.entries(src.headers as Record<string, unknown>).slice(0, 20)) {
        if (typeof v === 'string') out[k] = v.slice(0, 1024);
      }
      rules.headers = out;
    }
    patch.http_rules = rules;
  }

  if (Object.keys(patch).length === 0) return json({ error: 'no_changes' }, 400);

  try {
    const live = await updateProjectRuntime(session, project, patch);
    return json({ ok: true, live });
  } catch (err) {
    // Error REAL del control-plane (403 sin rol, 404, cuota del plan…): status real.
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
