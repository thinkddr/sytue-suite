/*
 * BFF de Sytue Admin — la apariencia por defecto de la organización: su color
 * (ADR 0071) y su forma.
 *
 *   PUT /api/admin/org/color   body { color }
 *   PUT /api/admin/org/forma   body { forma }
 *       Fija lo que ve quien no ha elegido lo suyo. Rol admin/owner.
 *
 * Proxy PURO, como el del logotipo: qué valores existen lo decide Sytue ID
 * (domain.OrgColors, domain.OrgFormas) y aquí no se repite la lista. Existe porque el
 * navegador nunca ve el access token.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { adminBaseUrl } from '../../../../lib/auth.js';

export const prerender = false;

const cabeceras = {
  'content-type': 'application/json; charset=utf-8',
  'cache-control': 'no-store',
};

/** Los ejes que Sytue ID sabe guardar; cualquier otro segmento no es de esta ruta. */
const EJES = new Set(['color', 'forma']);

export const PUT: APIRoute = async (ctx) => {
  const eje = ctx.params.eje ?? '';
  if (!EJES.has(eje)) {
    return new Response(JSON.stringify({ error: 'not_found' }), {
      status: 404,
      headers: cabeceras,
    });
  }
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return new Response(JSON.stringify({ error: 'forbidden' }), {
      status: guard.status === 403 ? 403 : 401,
      headers: cabeceras,
    });
  }
  try {
    const res = await fetch(`${adminBaseUrl()}/api/v1/admin/org/${eje}`, {
      method: 'PUT',
      headers: {
        authorization: `Bearer ${guard.session.tokens.accessToken}`,
        'content-type': 'application/json',
      },
      body: await ctx.request.text(),
    });
    return new Response(res.body, { status: res.status, headers: cabeceras });
  } catch {
    return new Response(JSON.stringify({ error: 'upstream' }), { status: 502, headers: cabeceras });
  }
};
