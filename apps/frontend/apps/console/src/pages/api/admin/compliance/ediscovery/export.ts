/*
 * BFF de Sytue Admin — export de eDiscovery (streaming).
 *
 *   GET /api/admin/compliance/ediscovery/export?service=mail|drive&email=&subject=&from=&to=&query=&case=
 *       Descarga el conjunto de resultados del sujeto: .mbox (Correo) o .zip (Archivos).
 *
 * Passthrough del stream del servicio (GET /api/v1/admin/ediscovery/export, Go): no
 * materializa el cuerpo, reenvía el ReadableStream y su Content-Disposition. Rol
 * admin/owner (el redirect de login es aceptable: es una navegación). El servicio
 * AUDITA el export (fail-closed) antes de emitir los bytes. Exportar datos ajenos es
 * privilegiado: la UI lo confirma antes de lanzar la descarga.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { ediscoveryExportResponse } from '../../../../../lib/compliance.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guard;

  const p = new URL(ctx.request.url).searchParams;
  const service = p.get('service');
  if (service !== 'mail' && service !== 'drive') {
    return new Response('Servicio inválido (mail|drive).', {
      status: 400,
      headers: { 'content-type': 'text/plain; charset=utf-8', 'cache-control': 'no-store' },
    });
  }

  const upstream = await ediscoveryExportResponse(guard.session, service, {
    email: p.get('email')?.trim() || '',
    subject: p.get('subject')?.trim() || '',
    from: p.get('from')?.trim() || '',
    to: p.get('to')?.trim() || '',
    query: p.get('query')?.trim() || '',
    case: p.get('case')?.trim() || '',
  }).catch(() => null);

  if (!upstream || !upstream.ok || !upstream.body) {
    return new Response('No se pudo exportar el conjunto de eDiscovery.', {
      status: 502,
      headers: { 'content-type': 'text/plain; charset=utf-8', 'cache-control': 'no-store' },
    });
  }

  const isMail = service === 'mail';
  return new Response(upstream.body, {
    status: 200,
    headers: {
      'content-type': isMail ? 'application/mbox' : 'application/zip',
      'content-disposition':
        upstream.headers.get('content-disposition') ??
        `attachment; filename="ediscovery.${isMail ? 'mbox' : 'zip'}"`,
      'cache-control': 'no-store',
    },
  });
};
