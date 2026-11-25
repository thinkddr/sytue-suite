/*
 * BFF de Sytue Admin — registro de solicitudes RGPD de la organización.
 *
 *   GET  /api/admin/privacy/requests[?status=open|received|…&kind=…&overdue=1]
 *        { requests, summary: { open, overdue } }.
 *   POST /api/admin/privacy/requests  body AltaSolicitud  →  201 { request }
 *        Registra una solicitud en nombre de una persona (de la organización, por
 *        su cuenta, o de fuera, por su correo).
 *
 * Proxya a Sytue ID (/api/v1/admin/privacy/requests, Go) con el access token
 * httpOnly; owner/admin, como allí. Propaga el código y el motivo del servicio.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { proxyError, proxyStatus } from '../../../../lib/admin.js';
import {
  CANALES,
  ESTADOS,
  TIPOS,
  validarAlta,
  type AltaSolicitud,
  type CanalSolicitud,
  type EstadoSolicitud,
  type TipoSolicitud,
} from '../../../../lib/privacidad.js';
import {
  crearSolicitud,
  listarSolicitudes,
  type FiltroRGPD,
} from '../../../../lib/privacidad-api.js';

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
  const p = new URL(ctx.request.url).searchParams;
  const status = p.get('status') ?? '';
  const kind = p.get('kind') ?? '';
  const filtro: FiltroRGPD = {};
  if (status === 'open' || (ESTADOS as readonly string[]).includes(status)) {
    filtro.status = status as 'open' | EstadoSolicitud;
  }
  if ((TIPOS as readonly string[]).includes(kind)) filtro.kind = kind as TipoSolicitud;
  if (p.get('overdue') === '1') filtro.overdue = true;
  try {
    return json(await listarSolicitudes(g.guard.session, filtro));
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};

export const POST: APIRoute = async (ctx) => {
  const g = await guardia(ctx);
  if ('fallo' in g) return g.fallo;
  const b = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const texto = (v: unknown): string | undefined =>
    typeof v === 'string' && v.trim() ? v.trim() : undefined;
  const alta: AltaSolicitud = {
    kind: (TIPOS as readonly string[]).includes(String(b?.kind))
      ? (b?.kind as TipoSolicitud)
      : ('' as TipoSolicitud),
    channel: (CANALES as readonly string[]).includes(String(b?.channel))
      ? (b?.channel as CanalSolicitud)
      : ('' as CanalSolicitud),
    identity_verified: b?.identity_verified === true,
  };
  const opcionales = [
    'user_id',
    'subject_email',
    'subject_name',
    'received_at',
    'identity_method',
    'notes',
  ] as const;
  for (const campo of opcionales) {
    const v = texto(b?.[campo]);
    if (v) alta[campo] = v;
  }
  const invalido = validarAlta(alta);
  if (invalido) return json({ error: 'bad_request', code: invalido }, 400);
  try {
    return json(await crearSolicitud(g.guard.session, alta), 201);
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
