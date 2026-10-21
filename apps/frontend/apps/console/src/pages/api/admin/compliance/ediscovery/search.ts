/*
 * BFF de Sytue Admin — búsqueda de eDiscovery sobre Correo y/o Archivos.
 *
 *   POST /api/admin/compliance/ediscovery/search
 *        body { scope, email, subject, from, to, query }
 *        Busca el contenido de un sujeto en los servicios del ámbito y reporta los
 *        resultados POR-SERVICIO (best-effort). Acceso privilegiado AUDITADO en el
 *        servicio (fail-closed: sin traza no hay resultado).
 *
 * Proxya a POST /api/v1/admin/ediscovery/search (Go) con el token httpOnly; rol
 * admin/owner. NO baja el contenido: solo metadatos + flag «preservado». Puro
 * passthrough + combinación best-effort.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { searchService, scopeServices, type SearchInput } from '../../../../../lib/compliance.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

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
  const email = typeof body?.email === 'string' ? body.email.trim() : '';
  const subject = typeof body?.subject === 'string' ? body.subject.trim() : '';
  if (!email && !subject) {
    return json({ error: 'bad_request', message: 'Indica el usuario objetivo.' }, 400);
  }
  // eDiscovery no tiene búsqueda org-wide: Archivos exige el sub del sujeto.
  if (services.includes('drive') && !subject) {
    return json(
      {
        error: 'bad_request',
        message: 'Para buscar en Archivos, elige el usuario del directorio.',
      },
      400,
    );
  }
  const input: SearchInput = {
    email,
    subject,
    from: typeof body?.from === 'string' ? body.from.trim() : '',
    to: typeof body?.to === 'string' ? body.to.trim() : '',
    query: typeof body?.query === 'string' ? body.query.trim() : '',
    case: typeof body?.case === 'string' ? body.case.trim() : '',
  };

  const results = await Promise.all(
    services.map(async (s) => [s, await searchService(guard.session, s, input)] as const),
  );
  return json(Object.fromEntries(results));
};
