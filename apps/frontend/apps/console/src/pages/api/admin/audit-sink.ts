/*
 * BFF de Sytue Admin — destino SIEM de la auditoría.
 *
 *   GET    /api/admin/audit-sink  -> configuración (sin secretos)
 *   PUT    /api/admin/audit-sink  -> guardar
 *   DELETE /api/admin/audit-sink  -> retirar el destino
 *
 * Proxya a /api/v1/admin/audit/sink de Sytue ID con el access token httpOnly.
 * Los secretos van SOLO de ida: el servicio nunca los devuelve, y este proxy
 * tampoco tendría cómo — que es exactamente lo que se quiere.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadAuditSink, saveAuditSink, deleteAuditSink, proxyStatus } from '../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function guard(ctx: Parameters<APIRoute>[0]) {
  return requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    // Configurar a dónde se envía la auditoría es gobernar la auditoría, no
    // leerla: quien solo puede verla no puede además redirigirla a otro sitio.
    perms: ['audit.view'],
  });
}

export const GET: APIRoute = async (ctx) => {
  const g = await guard(ctx);
  if (g instanceof Response) return json({ error: 'forbidden' }, g.status === 403 ? 403 : 401);
  // Sin destino configurado devuelve null, que NO es un fallo: es el estado
  // inicial de toda organización.
  const sink = await loadAuditSink(g.session);
  return json({ sink });
};

export const PUT: APIRoute = async (ctx) => {
  const g = await guard(ctx);
  if (g instanceof Response) return json({ error: 'forbidden' }, g.status === 403 ? 403 : 401);
  // El cuerpo lo valida el servicio (URL https, sin direcciones internas); aquí
  // solo se le da forma para no propagar un `any` hasta la llamada.
  const body = (await ctx.request.json().catch(() => ({}))) as {
    url?: string;
    auth_header?: string;
    signing_secret?: string;
    enabled?: boolean;
  };
  try {
    return json({ sink: await saveAuditSink(g.session, { ...body, url: body.url ?? '' }) });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const g = await guard(ctx);
  if (g instanceof Response) return json({ error: 'forbidden' }, g.status === 403 ? 403 : 401);
  try {
    await deleteAuditSink(g.session);
    return json({ ok: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
