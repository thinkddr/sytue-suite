/*
 * POST /api/services/create { project, kind, name?, app_id? } -> aprovisiona un
 * servicio gestionado (Postgres, Storage, Functions, Cron o una app del catálogo)
 * en el proyecto.
 *
 * BFF de aprovisionamiento. Proxy PURO al control-plane (POST
 * /api/v1/projects/{id}/services); sin lógica (invariante). El SDK ya tenía el
 * método y no lo llamaba NADIE: el menú del proyecto ofrecía «Base de datos» sin
 * que hubiera forma de crear una desde la web. Devuelve { ok, live, service }:
 * live=false cuando no hay backend vivo y el alta queda en demostración. La
 * autorización real (rol admin/owner, cuota del plan) la impone el control-plane.
 * NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import type { ManagedServiceKind } from '@sytue/api-clients';
import { bff, toBffContext } from '../../../lib/auth.js';
import { createService, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

/** Los que el control-plane sabe aprovisionar (domain.ServiceKind). */
const KINDS = ['postgres', 'storage', 'functions', 'cron', 'job', 'worker', 'app'] as const;
type Kind = (typeof KINDS)[number];

/** Nombre por defecto legible; el control-plane acepta cualquiera no vacío. */
const DEFAULT_NAME: Record<Kind, string> = {
  postgres: 'Sytue Postgres',
  storage: 'Sytue Storage',
  functions: 'Sytue Functions',
  cron: 'Sytue Cron',
  job: 'Sytue Job',
  worker: 'Sytue Worker',
  app: 'App',
};

/**
 * Claves de `config` que cada tipo puede traer. Es una lista BLANCA por tipo, no
 * un `config` libre: el mapa va tal cual al control-plane, así que dejar pasar
 * cualquier clave desde el navegador sería regalar la configuración interna del
 * aprovisionamiento (imagen, comando, host…) a quien no debería tocarla.
 *
 * Lo que aquí se comprueba es la FORMA. Lo que significa cada valor lo valida el
 * control-plane, que es donde está el criterio: la imagen tiene que vivir en el
 * repo del propio proyecto (aislamiento entre tenants), la URL de un cron pasa
 * por el filtro anti-SSRF y el `schedule` por el parser de cron. Duplicar esas
 * reglas aquí solo produciría dos verdades que se separan con el tiempo.
 */
const CONFIG_KEYS: Partial<Record<Kind, readonly string[]>> = {
  functions: ['image', 'command'],
  cron: ['schedule', 'url', 'method', 'image', 'command'],
  job: ['image', 'command'],
  worker: ['image', 'command', 'replicas'],
  // La copia diaria se hace a las 3:00 salvo que se diga otra cosa. Se elige al
  // crear la base porque hoy la configuración de un servicio no se puede editar
  // después: cuando exista `PATCH /services/{id}`, este campo se moverá también
  // a la pantalla de la base de datos.
  postgres: ['backup_schedule'],
};

/**
 * Id del catálogo: minúsculas, dígitos y guiones. Se comprueba la FORMA, no la
 * existencia — quién existe lo decide el control-plane contra el catálogo
 * embebido, y duplicar esa lista aquí solo la dejaría vieja.
 */
const APP_ID = /^[a-z][a-z0-9-]{1,30}[a-z0-9]$/;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    project?: unknown;
    kind?: unknown;
    name?: unknown;
    app_id?: unknown;
    config?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const project = typeof body.project === 'string' ? body.project.trim() : '';
  const kind = typeof body.kind === 'string' ? body.kind.trim() : '';
  if (!project) return json({ error: 'missing_project' }, 400);
  if (!KINDS.includes(kind as Kind)) return json({ error: 'invalid_kind' }, 400);

  const name = (typeof body.name === 'string' ? body.name.trim() : '').slice(0, 120);

  // Una app del catálogo se elige por su id y NADA más: no hay forma de mandar
  // una imagen desde aquí, que es el invariante entero de la ADR 0029.
  let config: Record<string, string> | undefined;
  if (kind === 'app') {
    const appId = typeof body.app_id === 'string' ? body.app_id.trim() : '';
    if (!APP_ID.test(appId)) return json({ error: 'invalid_app_id' }, 400);
    config = { app_id: appId };
  } else {
    config = pickConfig(kind as Kind, body.config);
    // Lo mínimo sin lo que el alta no puede salir bien, comprobado aquí para que
    // el formulario reciba un error suyo en vez del 400 genérico de la API: una
    // función/job necesita QUÉ ejecutar, y un cron CUÁNDO.
    if ((kind === 'functions' || kind === 'job') && !config?.image) {
      return json({ error: 'missing_image' }, 400);
    }
    if (kind === 'cron') {
      if (!config?.schedule) return json({ error: 'missing_schedule' }, 400);
      if (!config.url && !config.image) return json({ error: 'missing_target' }, 400);
    }
  }

  try {
    const res = await createService(session, project, {
      kind: kind as ManagedServiceKind,
      name: name || DEFAULT_NAME[kind as Kind],
      ...(config ? { config } : {}),
    });
    return json({ ok: true, live: res.live, service: res.service });
  } catch (err) {
    // Error REAL del control-plane (403 sin rol, 402 cuota, 409 ya existe…):
    // status y código reales a la UI.
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    throw err;
  }
};

/**
 * Copia de `raw` solo las claves que el tipo admite, ya recortadas. Devuelve
 * undefined si no queda ninguna, para no mandar un `config: {}` que el
 * control-plane tendría que interpretar.
 */
function pickConfig(kind: Kind, raw: unknown): Record<string, string> | undefined {
  const allowed = CONFIG_KEYS[kind];
  if (!allowed || typeof raw !== 'object' || raw === null) return undefined;
  const src = raw as Record<string, unknown>;
  const out: Record<string, string> = {};
  for (const key of allowed) {
    const v = src[key];
    // 2 KiB por valor: un comando largo cabe de sobra y una cadena de megabytes
    // no llega a viajar. El tope real lo pone igualmente el control-plane.
    if (typeof v === 'string' && v.trim() !== '') out[key] = v.trim().slice(0, 2048);
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
