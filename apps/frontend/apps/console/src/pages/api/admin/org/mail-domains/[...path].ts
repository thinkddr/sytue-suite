/*
 * BFF de Sytue Admin — LOS dominios de correo de la organización.
 *
 *   GET    /api/admin/org/mail-domains            -> { items, mail_domain, base_domain }
 *   POST   /api/admin/org/mail-domains  { domain } -> añade y comprueba el DNS
 *   POST   /api/admin/org/mail-domains/<id>/primary -> estrena ese dominio
 *   DELETE /api/admin/org/mail-domains/<id>        -> lo desconecta (204)
 *
 * Proxya a Sytue ID (/api/v1/admin/mail-domains) con el access token httpOnly.
 * Rol admin/owner: los dominios deciden la dirección de toda la plantilla.
 *
 * Va con `[...path]` y no como cuatro ficheros porque las cuatro operaciones son
 * la misma pantalla y comparten guardia y traducción de errores; la ruta que
 * llega se acota abajo contra una lista, de modo que esto no es un proxy abierto
 * a /api/v1/admin.
 *
 * El `message` del servicio se conserva en los errores. Aquí no es un detalle de
 * estilo: las respuestas útiles de esta pantalla son justo los mensajes —«otra
 * organización ya verificó este dominio», «eso no parece un dominio»— y aplanarlos
 * a «error» deja a alguien probando a ciegas contra su propio DNS.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import {
  claimOrgMailDomain,
  deleteOrgMailDomain,
  loadOrgMailDomains,
  proxyStatus,
  setPrimaryOrgMailDomain,
} from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function guardAdmin(ctx: Parameters<APIRoute>[0]) {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  return guard;
}

/** El mensaje del servicio, si vino en el sobre canónico de httpx. */
function upstreamMessage(e: unknown): string | undefined {
  const body = (e as { body?: { error?: { message?: string }; message?: string } })?.body;
  return body?.error?.message ?? body?.message;
}

/** El identificador de la ruta, solo si es un UUID: nada más llega al servicio. */
function domainId(path: string | undefined, sufijo = ''): string | null {
  const partes = (path ?? '').split('/').filter(Boolean);
  const esperadas = sufijo ? 2 : 1;
  if (partes.length !== esperadas) return null;
  if (sufijo && partes[1] !== sufijo) return null;
  const id = partes[0] ?? '';
  return /^[0-9a-f-]{36}$/i.test(id) ? id : null;
}

export const GET: APIRoute = async (ctx) => {
  const guard = await guardAdmin(ctx);
  if (guard instanceof Response) return guard;
  if (ctx.params.path) return json({ error: 'not_found' }, 404);
  const { data, live } = await loadOrgMailDomains(guard.session);
  if (!live) return json({ error: 'upstream' }, 503);
  return json(data);
};

export const POST: APIRoute = async (ctx) => {
  const guard = await guardAdmin(ctx);
  if (guard instanceof Response) return guard;
  try {
    // .../<id>/primary — estrenar un dominio ya conectado.
    const id = domainId(ctx.params.path, 'primary');
    if (id) return json(await setPrimaryOrgMailDomain(guard.session, id));
    if (ctx.params.path) return json({ error: 'not_found' }, 404);
    // Sin ruta: añadir (o recomprobar) un dominio.
    const body = (await ctx.request.json().catch(() => null)) as { domain?: unknown } | null;
    const host = typeof body?.domain === 'string' ? body.domain.trim() : '';
    if (!host) return json({ error: 'bad_request', message: 'Falta el dominio.' }, 400);
    return json(await claimOrgMailDomain(guard.session, host));
  } catch (e) {
    return json({ error: 'upstream', message: upstreamMessage(e) }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await guardAdmin(ctx);
  if (guard instanceof Response) return guard;
  const id = domainId(ctx.params.path);
  if (!id) return json({ error: 'not_found' }, 404);
  try {
    await deleteOrgMailDomain(guard.session, id);
    return new Response(null, { status: 204 });
  } catch (e) {
    return json({ error: 'upstream', message: upstreamMessage(e) }, proxyStatus(e));
  }
};
