/*
 * BFF de Sytue Admin — permisos de Sytue ERP de una persona.
 *
 *   PUT /api/admin/erp-permisos   { sub, email, nombre, modulos }
 *
 * Solo la escritura: el listado se sirve en el render de la página. Proxya a
 * `services/syerp` (PUT /api/v1/admin/permisos/{sub}) con el token de la sesión;
 * quién puede lo decide el ERP y aquí se exige el mismo rol para no ofrecer un
 * botón que acabe en 403.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { saveErpPermisos } from '../../../lib/erp.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

const texto = (v: unknown): v is string => typeof v === 'string';

export const PUT: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
  });
  if (guard instanceof Response) {
    return json({ error: guard.status === 403 ? 'forbidden' : 'unauthenticated' }, guard.status);
  }

  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const modulos = body?.modulos;
  if (
    !texto(body?.sub) ||
    !body.sub ||
    !texto(body.email) ||
    !texto(body.nombre) ||
    typeof modulos !== 'object' ||
    modulos === null ||
    !Object.values(modulos).every(texto)
  ) {
    return json({ error: 'bad_request' }, 400);
  }

  // El código del ERP se propaga: 422 (un módulo o nivel que no existe), 403 y 502
  // son mensajes distintos.
  const res = await saveErpPermisos(guard.session, {
    sub: body.sub,
    email: body.email,
    nombre: body.nombre,
    modulos: modulos as Record<string, string>,
  });
  return json(res.ok ? { ok: true } : { error: 'upstream' }, res.ok ? 200 : res.status);
};
