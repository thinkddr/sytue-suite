/*
 * BFF de Sytue Admin — retenciones legales (legal hold) de Correo, Archivos y
 * Notepads.
 *
 *   GET  /api/admin/compliance/legal-holds[?include_released=true]
 *        Lista holds de los tres servicios, reportados POR-SERVICIO (best-effort).
 *   POST /api/admin/compliance/legal-holds  body { scope, orgWide, subject, email, reason, case }
 *        Crea el hold en los servicios del ámbito; reporta el resultado por-servicio.
 *
 * Proxya a las APIs admin de Mail/Drive/Notes (Go) con el access token httpOnly;
 * rol admin/owner (el servicio re-verifica el rol, aísla por org y AUDITA). Puro
 * passthrough + combinación best-effort: si un servicio falla, los demás siguen.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  loadHolds,
  createServiceHold,
  scopeServices,
  type CreateHoldInput,
} from '../../../../lib/compliance.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const includeReleased = new URL(ctx.request.url).searchParams.get('include_released') === 'true';
  return json(await loadHolds(guard.session, includeReleased));
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);

  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const services = scopeServices(body?.scope);
  if (services.length === 0) {
    return json(
      { error: 'bad_request', message: 'Ámbito inválido (correo, archivos o ambos).' },
      400,
    );
  }
  const orgWide = body?.orgWide === true;
  const subject = typeof body?.subject === 'string' ? body.subject.trim() : '';
  const email = typeof body?.email === 'string' ? body.email.trim() : '';
  if (!orgWide && !subject && !email) {
    return json(
      { error: 'bad_request', message: 'Indica un usuario objetivo o marca toda la organización.' },
      400,
    );
  }
  // Archivos y Notepads solo saben resolver por sub: sin él (y sin org-wide) no
  // se puede retener. Correo es el único que además acepta email.
  if (!orgWide && (services.includes('drive') || services.includes('notes')) && !subject) {
    return json(
      {
        error: 'bad_request',
        message: 'Para retener Archivos o Notepads, elige el usuario del directorio.',
      },
      400,
    );
  }
  const input: CreateHoldInput = {
    orgWide,
    subject,
    email,
    reason: typeof body?.reason === 'string' ? body.reason.trim() : '',
    case: typeof body?.case === 'string' ? body.case.trim() : '',
  };

  const results = await Promise.all(
    services.map(async (s) => [s, await createServiceHold(guard.session, s, input)] as const),
  );
  const anyOk = results.some(([, r]) => r.ok);
  return json(Object.fromEntries(results), anyOk ? 201 : 502);
};
