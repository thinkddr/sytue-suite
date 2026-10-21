/*
 * BFF de Sytue Admin — invitar usuario a la organización activa.
 *
 *   GET  /api/admin/invitations   → { items: [{ email, role, expires_at }] }
 *       Las invitaciones pendientes de la organización activa. Rol admin/owner.
 *
 *   DELETE /api/admin/invitations  body { email: string }
 *       Revoca la invitación pendiente de ese correo. Idempotente: si ya no
 *       había ninguna, contesta { revoked: false } y 200, no un error.
 *
 *   POST /api/admin/invitations   body { email: string, role: SytueRole }
 *       Emite una invitación: Sytue ID genera un token de un solo uso y envía el
 *       enlace de aceptación por email. Rol admin/owner.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/invitations, Go) con el access token
 * httpOnly; propaga el status del servicio (403/409/400/502). El token nunca sale
 * al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { inviteUser, listInvitations, proxyStatus, revokeInvitation } from '../../../lib/admin.js';
import type { SytueRole } from '../../../lib/types.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

const ROLES: readonly SytueRole[] = ['owner', 'admin', 'member', 'billing'];

/** El guard es el mismo para los tres métodos: quien invita es quien puede ver y quitar. */
async function guardia(ctx: Parameters<APIRoute>[0]) {
  return requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.manage'],
  });
}

function sinSesion(guard: Response): Response {
  return json(
    { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
    guard.status === 403 ? 403 : 401,
  );
}

export const GET: APIRoute = async (ctx) => {
  const guard = await guardia(ctx);
  if (guard instanceof Response) return sinSesion(guard);
  try {
    return json({ items: await listInvitations(guard.session) });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await guardia(ctx);
  if (guard instanceof Response) return sinSesion(guard);
  const body = (await ctx.request.json().catch(() => null)) as { email?: unknown } | null;
  const email = typeof body?.email === 'string' ? body.email.trim() : '';
  if (!email) return json({ error: 'bad_request', message: 'Falta el correo.' }, 400);
  try {
    return json(await revokeInvitation(guard.session, email));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const body = (await ctx.request.json().catch(() => null)) as {
    email?: unknown;
    role?: unknown;
    mailbox_local?: unknown;
    password?: unknown;
  } | null;
  const email = typeof body?.email === 'string' ? body.email.trim() : '';
  const role = body?.role;
  // Espejo mínimo de la validación para el UX; la política real la aplica Sytue ID.
  if (!email || !email.includes('@')) {
    return json({ error: 'bad_request', message: 'Introduce un correo válido.' }, 400);
  }
  if (typeof role !== 'string' || !ROLES.includes(role as SytueRole)) {
    return json({ error: 'bad_request', message: 'Rol inválido.' }, 400);
  }
  try {
    // El buzón elegido y la contraseña se reenvían TAL CUAL. Los valida Sytue ID
    // —la dirección contra el dominio de la organización, la contraseña contra su
    // política—, y duplicar aquí esas reglas solo podría divergir de las de
    // verdad. Este proxy se limitaba a email y rol, así que la dirección que el
    // administrador escribía se perdía por el camino sin que nada fallara.
    const mailbox = typeof body?.mailbox_local === 'string' ? body.mailbox_local.trim() : '';
    const password = typeof body?.password === 'string' ? body.password : '';
    return json(await inviteUser(guard.session, email, role as SytueRole, mailbox, password), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
