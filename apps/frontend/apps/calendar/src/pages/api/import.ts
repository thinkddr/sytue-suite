/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /api/import — «Traer mis datos» de Sytue Calendar: importar los calendarios y sus eventos desde Google
 * Workspace o Microsoft 365.
 *
 *   GET  /api/import?domains=…  → { origins, jobs }  orígenes conectados + mi progreso
 *   POST /api/import { instanceId, source, domains } → lanza la importación
 *
 * Proxy BFF al Sytue Integration Gateway con el access token resuelto en el SERVIDOR: el
 * token nunca toca el navegador. El gateway ignora cualquier identidad que se le mande —
 * destino y origen son siempre quien llama—, así que esta puerta no puede tocar los datos
 * de otra persona.
 *
 * La lógica compartida vive en @sytue/api-clients: los cuatro productos necesitan
 * exactamente lo mismo y cuatro copias solo divergirían.
 */
import type { APIRoute } from 'astro';
import { listImportOrigins, listMyImports, startImport } from '@sytue/api-clients';
import type { ImportDomain } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session';

export const prerender = false;

/** Contenido que ESTE producto sabe traer. */
const PROPIOS: ImportDomain[] = ['calendar'];

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Base del gateway. Vacía ⇒ no hay de dónde traer y la interfaz no ofrece nada.
 *
 *  `serverEnv` porque en el SSR compilado `import.meta.env` va congelado del build:
 *  sin esto la variable nunca se lee y siempre gana el valor por defecto, que solo
 *  acierta en producción y por casualidad. */
function gatewayBase(): string {
  const env = serverEnv(import.meta.env);
  return String(env.SYTUE_INTEGRATIONS_BASE_URL ?? 'https://integrations.sytue.eu');
}

/** Solo lo que este producto trae: lo que pida el cliente se filtra contra PROPIOS. */
function pedidos(raw: string | null): ImportDomain[] {
  const piden = new Set((raw ?? '').split(',').map((d) => d.trim()));
  const out = PROPIOS.filter((d) => piden.has(d));
  return out.length > 0 ? out : PROPIOS;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const base = gatewayBase();
  const [origins, jobs] = await Promise.all([
    listImportOrigins(base, session.accessToken),
    listMyImports(base, session.accessToken, pedidos(ctx.url.searchParams.get('domains'))),
  ]);
  return json({ origins, jobs });
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let p: { instanceId?: unknown; source?: unknown; domains?: unknown };
  try {
    p = (await ctx.request.json()) as typeof p;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const instanceId = typeof p.instanceId === 'string' ? p.instanceId : '';
  const source = p.source === 'google' || p.source === 'm365' ? p.source : null;
  if (!instanceId || !source) return json({ error: 'missing_fields' }, 400);

  // Los tipos de contenido se ACOTAN a los de este producto: que el navegador pida más
  // no debe convertir «traer mi correo» en «traer todo lo mío».
  const piden = Array.isArray(p.domains) ? p.domains.map(String) : [];
  const domains = PROPIOS.filter((d) => piden.includes(d));
  if (domains.length === 0) return json({ error: 'missing_domains' }, 400);

  const res = await startImport(gatewayBase(), session.accessToken, {
    instanceId,
    source,
    domains,
  });
  if (!res.ok) return json({ error: 'import_failed' }, res.status >= 500 ? 502 : res.status);
  return json({ jobs: res.jobs }, 202);
};
