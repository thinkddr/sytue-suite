/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /api/embed  (application/json) — plano de PRESENTACIÓN L4 del bloque «Informe de
 * Power BI» del editor. Proxy BFF autorizado por la cookie de sesión httpOnly al
 * Sytue Integration Gateway (services/integrations), con el access token resuelto en
 * el SERVIDOR (passthrough del JWT del usuario — mismo patrón que el conector
 * KindGateway de docs). El token NUNCA llega al navegador; el artefacto de embed lo
 * mintea el gateway con el service principal de la instancia.
 *
 *   GET  /api/embed  → instancias Power BI conectadas → { instances: [{id,name,status}] }
 *   POST /api/embed  → mint del artefacto acotado (server-side) → { embed_url, token,
 *                      token_type, expires_at }.  body: { instance_id, report_id, group_id? }
 *
 * El token minted se reenvía VERBATIM al llamante (isla → iframe por postMessage) con
 * cache-control:no-store; jamás se cachea, loguea ni persiste aquí. El content-type
 * application/json evita el checkOrigin (CSRF) de Astro.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, integrationsBaseUrl } from '../../lib/auth';

export const prerender = false;

/** El cuerpo es diminuto (ids + report_id): se rechaza antes de leer. */
const MAX_BYTES = 8 * 1024;

/** UUID canónico 8-4-4-4-12 (frontera de confianza antes de interpolar en la ruta). */
const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

const gwBase = (): string => integrationsBaseUrl().replace(/\/$/, '');

interface WireInstance {
  id: string;
  name: string;
  template_key: string;
  status: string;
}

/** Instancias Power BI conectadas de la org, para el selector del bloque. Degrada a
 *  lista vacía si el gateway no responde (el bloque muestra «conecta Power BI»). */
export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await fetch(`${gwBase()}/api/v1/instances`, {
      headers: { authorization: `Bearer ${session.tokens.accessToken}` },
    });
    if (!res.ok) return json({ instances: [] });
    const wire = (await res.json()) as { instances?: WireInstance[] };
    const instances = (wire.instances ?? [])
      .filter((i) => i.template_key === 'powerbi')
      .map((i) => ({ id: i.id, name: i.name, status: i.status }));
    return json({ instances });
  } catch {
    return json({ instances: [] });
  }
};

/** Mint del artefacto de embed acotado (server-side). Reenvía report_id/group_id al
 *  gateway; devuelve su respuesta verbatim (el token viaja al llamante, no se toca). */
export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const len = Number(ctx.request.headers.get('content-length') ?? '0');
  if (Number.isFinite(len) && len > MAX_BYTES) return json({ error: 'bad_request' }, 413);

  let payload: { instance_id?: string; report_id?: string; group_id?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const instanceId = (payload.instance_id ?? '').trim();
  if (!UUID_RE.test(instanceId)) return json({ error: 'bad_instance' }, 400);
  const forward = JSON.stringify({
    report_id: payload.report_id ?? '',
    group_id: payload.group_id ?? '',
  });

  try {
    const res = await fetch(
      `${gwBase()}/api/v1/instances/${encodeURIComponent(instanceId)}/embed/token`,
      {
        method: 'POST',
        headers: {
          authorization: `Bearer ${session.tokens.accessToken}`,
          'content-type': 'application/json',
        },
        body: forward,
      },
    );
    const body = res.status === 204 ? null : await res.text();
    return new Response(body, {
      status: res.status,
      headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
    });
  } catch {
    return json({ error: 'upstream_unreachable' }, 502);
  }
};
