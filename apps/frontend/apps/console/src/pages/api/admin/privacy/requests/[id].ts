/*
 * BFF de Sytue Admin — una solicitud RGPD.
 *
 *   GET   /api/admin/privacy/requests/:id   { request, events, erasure }
 *   PATCH /api/admin/privacy/requests/:id   body CambiosSolicitud → { request }
 *         Avanzarla o cerrarla (cerrar exige resolución: es lo que se contesta).
 *
 * Proxya a Sytue ID con el token httpOnly; owner/admin. Propaga el motivo del
 * servicio (una cerrada no cambia; una supresión en curso no se da por resuelta).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { proxyError, proxyStatus } from '../../../../../lib/admin.js';
import { ESTADOS, type EstadoSolicitud } from '../../../../../lib/privacidad.js';
import {
  cambiarSolicitud,
  verSolicitud,
  type CambiosSolicitud,
} from '../../../../../lib/privacidad-api.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function guardia(ctx: Parameters<APIRoute>[0]) {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return {
      fallo: json(
        { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
        guard.status === 403 ? 403 : 401,
      ),
    } as const;
  }
  return { guard } as const;
}

export const GET: APIRoute = async (ctx) => {
  const g = await guardia(ctx);
  if ('fallo' in g) return g.fallo;
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    return json(await verSolicitud(g.guard.session, id));
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};

export const PATCH: APIRoute = async (ctx) => {
  const g = await guardia(ctx);
  if ('fallo' in g) return g.fallo;
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  const b = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const cambios: CambiosSolicitud = {};
  if (typeof b?.status === 'string') {
    if (!(ESTADOS as readonly string[]).includes(b.status)) {
      return json({ error: 'bad_request', code: 'status' }, 400);
    }
    cambios.status = b.status as EstadoSolicitud;
  }
  if (typeof b?.resolution === 'string') cambios.resolution = b.resolution;
  if (typeof b?.notes === 'string') cambios.notes = b.notes;
  if (typeof b?.identity_method === 'string') cambios.identity_method = b.identity_method;
  if (typeof b?.identity_verified === 'boolean') cambios.identity_verified = b.identity_verified;
  try {
    return json(await cambiarSolicitud(g.guard.session, id, cambios));
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
