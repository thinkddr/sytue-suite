/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /api/gateway-instances  (application/json) — aplicaciones YA CONECTADAS de la
 * organización que pueden servir datos tabulares a Sytue Sheets, para elegirlas de
 * una LISTA en el asistente de «Traer datos de fuera» (origen tipo `gateway` del
 * conector de docs). Sin esto, la única forma de usar ese origen sería pegar a mano
 * el identificador de la instancia, que es justo lo que no queremos.
 *
 *   GET /api/gateway-instances → { instances: [{ id, name, app, driver }] }
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly al Sytue Integration
 * Gateway (services/integrations) con el access token resuelto en el SERVIDOR
 * (passthrough del JWT del usuario, mismo patrón que /api/embed y que el conector
 * KindGateway de docs). El token NUNCA llega al navegador.
 *
 * Se cruzan dos lecturas del gateway (ambas de miembro, sin privilegios de admin):
 *   GET /api/v1/instances  → las conexiones de la org (estado + capacidades)
 *   GET /api/v1/templates  → el catálogo; `manifest.data` es el DRIVER tabular de
 *                            la app ("" = esa app no expone datos)
 * y solo se devuelven las ACTIVAS con la capacidad de datos habilitada. El driver
 * viaja porque determina cómo se escribe la consulta (entity set, JQL, SOQL, SQL…).
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, integrationsBaseUrl } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

interface WireInstance {
  id: string;
  name: string;
  template_key: string;
  status: string;
  /** Capacidades habilitadas; vacío = todas (contrato del gateway). */
  level_enabled?: string[];
}

interface WireTemplate {
  key: string;
  manifest?: { name?: string; data?: string };
}

/**
 * ¿Esta instancia puede traer datos AHORA? Activa y con la capacidad `data`
 * habilitada — lista vacía significa «todas» en el gateway, así que también pasa.
 */
function servesData(i: WireInstance): boolean {
  const caps = i.level_enabled ?? [];
  return i.status === 'active' && (caps.length === 0 || caps.includes('data'));
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const base = integrationsBaseUrl().replace(/\/$/, '');
  const headers = { authorization: `Bearer ${session.tokens.accessToken}` };
  try {
    const [instRes, tplRes] = await Promise.all([
      fetch(`${base}/api/v1/instances`, { headers }),
      fetch(`${base}/api/v1/templates`, { headers }),
    ]);
    // Degrada a lista vacía: el asistente dirá que no hay ninguna conectada.
    if (!instRes.ok || !tplRes.ok) return json({ instances: [] });

    const { instances: wire = [] } = (await instRes.json()) as { instances?: WireInstance[] };
    const { templates = [] } = (await tplRes.json()) as { templates?: WireTemplate[] };
    const byKey = new Map(templates.map((t) => [t.key, t]));

    const instances = wire.filter(servesData).flatMap((i) => {
      const tpl = byKey.get(i.template_key);
      const driver = (tpl?.manifest?.data ?? '').trim();
      if (driver === '') return []; // esa aplicación no expone datos tabulares
      return [{ id: i.id, name: i.name, app: tpl?.manifest?.name ?? i.template_key, driver }];
    });
    return json({ instances });
  } catch {
    return json({ instances: [] });
  }
};
