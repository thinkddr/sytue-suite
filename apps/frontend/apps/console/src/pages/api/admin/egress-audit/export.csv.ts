/*
 * BFF de Sytue Admin — exportación CSV del registro de egress (streaming).
 *
 *   GET /api/admin/egress-audit/export.csv[?host=&status=&since=&until=]
 *       Descarga CSV del registro de egress filtrado de la organización activa.
 *
 * Passthrough del stream del gateway (GET /api/v1/admin/egress-audit?format=csv,
 * Go): no materializa el cuerpo, reenvía el ReadableStream y su Content-Disposition.
 * Exige rol admin/owner (el redirect de login es aceptable: es una navegación).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { egressExportResponse } from '../../../../lib/gateway.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guard;

  const p = new URL(ctx.request.url).searchParams;
  const upstream = await egressExportResponse(guard.session, {
    host: p.get('host')?.trim() || undefined,
    status: p.get('status')?.trim() || undefined,
    since: p.get('since')?.trim() || undefined,
    until: p.get('until')?.trim() || undefined,
  }).catch(() => null);

  if (!upstream || !upstream.ok || !upstream.body) {
    return new Response('No se pudo exportar el registro de egress.', {
      status: 502,
      headers: { 'content-type': 'text/plain; charset=utf-8', 'cache-control': 'no-store' },
    });
  }
  return new Response(upstream.body, {
    status: 200,
    headers: {
      'content-type': 'text/csv; charset=utf-8',
      'content-disposition':
        upstream.headers.get('content-disposition') ?? 'attachment; filename="egress.csv"',
      'cache-control': 'no-store',
    },
  });
};
