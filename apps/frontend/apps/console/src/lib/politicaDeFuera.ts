/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El BFF de una política de la organización con gente de otras organizaciones
 * (`components/PoliticaDeFuera.tsx`): el de las peticiones de firma de CertiPDF
 * (Docs, ADR 0059) y el de las conversaciones de Rooms (mig 0083 de Rooms).
 *
 *   GET  → { mode, trusted }
 *   POST { mode, trusted } → la guarda (PUT en el servicio)
 *
 * Proxya con el access token httpOnly de la sesión. Quién puede lo decide el servicio
 * —owner o admin— y la organización es la del token; aquí se exige el mismo rol para
 * no ofrecer un botón que va a acabar en 403. La escritura va por POST: el checkOrigin
 * de Astro rechaza PUT tras el proxy (mismo patrón que el resto de la consola).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../server/bff.js';

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function guard(ctx: Parameters<APIRoute>[0]) {
  return requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
  });
}

/** El error del servicio (`{"error":{code,details}}`) aplanado, con su código HTTP. */
async function respuesta(res: Response): Promise<Response> {
  const cuerpo = (await res.json().catch(() => ({}))) as {
    error?: { code?: string; details?: unknown };
  };
  if (res.ok) return json(cuerpo);
  return json(
    { error: cuerpo.error?.code ?? 'upstream', details: cuerpo.error?.details ?? null },
    res.status,
  );
}

/** Las dos rutas del BFF de una política, contra `ruta()` del servicio. */
export function rutasDePolitica(ruta: () => string): { GET: APIRoute; POST: APIRoute } {
  const GET: APIRoute = async (ctx) => {
    const g = await guard(ctx);
    if (g instanceof Response) return json({ error: 'forbidden' }, g.status);
    const res = await fetch(ruta(), {
      headers: {
        authorization: `Bearer ${g.session.tokens.accessToken}`,
        accept: 'application/json',
      },
    }).catch(() => null);
    return res ? respuesta(res) : json({ error: 'upstream' }, 502);
  };

  const POST: APIRoute = async (ctx) => {
    const g = await guard(ctx);
    if (g instanceof Response) return json({ error: 'forbidden' }, g.status);
    const p = (await ctx.request.json().catch(() => null)) as {
      mode?: unknown;
      trusted?: unknown;
    } | null;
    const trusted = Array.isArray(p?.trusted)
      ? p.trusted.filter((x): x is string => typeof x === 'string')
      : [];
    if (typeof p?.mode !== 'string') return json({ error: 'bad_request' }, 400);
    const res = await fetch(ruta(), {
      method: 'PUT',
      headers: {
        authorization: `Bearer ${g.session.tokens.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify({ mode: p.mode, trusted }),
    }).catch(() => null);
    return res ? respuesta(res) : json({ error: 'upstream' }, 502);
  };

  return { GET, POST };
}
