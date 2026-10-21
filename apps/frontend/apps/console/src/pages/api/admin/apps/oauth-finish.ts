/*
 * BFF de Sytue Admin — terminar el consentimiento OAuth de una conexión.
 *
 *   GET /api/admin/apps/oauth-finish?state=…&code=…&error=…
 *
 * Aquí llega el navegador al volver del proveedor, reenviado por el servicio de
 * integraciones desde `/api/v1/oauth/callback` (la URI registrada en cada proveedor,
 * que no cambia). Aquella vuelta no lleva sesión y por eso ya no canjea nada: el
 * canje se pide desde aquí, con la sesión de quien vuelve, y el servicio solo lo
 * hace si es la persona que empezó el consentimiento (NEG-10). Sin sesión, el guard
 * manda a iniciar sesión y vuelve aquí con la misma query.
 *
 * Termina siempre en una redirección a la sección de la consola con el desenlace.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { finishOAuth } from '../../../../lib/gateway.js';
import {
  destinoTrasConsentimiento,
  type DesenlaceConsentimiento,
} from '../../../../lib/consentimiento.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const bff = getBff();
  const guard = await requireSession(bff, toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response) return guard;

  const q = ctx.url.searchParams;
  let desenlace: DesenlaceConsentimiento;
  try {
    desenlace = await finishOAuth(guard.session, {
      state: q.get('state') ?? '',
      code: q.get('code') ?? '',
      error: q.get('error') ?? '',
    });
  } catch {
    desenlace = { error: 'exchange' };
  }
  return new Response(null, {
    status: 302,
    headers: {
      location: destinoTrasConsentimiento(bff.basePath, desenlace),
      'cache-control': 'no-store',
    },
  });
};
