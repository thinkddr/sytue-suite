/*
 * BFF de Sytue Admin — migración de contenido desde Microsoft 365 / Google.
 *
 *   GET  /api/admin/migrations   Trabajos de la organización (para el progreso en vivo).
 *   POST /api/admin/migrations   Lanza una tanda: personas × tipos de contenido.
 *
 * Proxya al Sytue Integration Gateway (/api/v1/migrations, Go) con el access token
 * httpOnly; propaga el status del servicio. Rol admin/owner. Sin lógica propia: el
 * gateway valida, aísla por org (RLS) y canjea el permiso de cada persona.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import {
  createMigrations,
  loadMigrations,
  proxyStatus,
  sanitizeMigrationInput,
} from '../../../lib/gateway.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function guardOf(ctx: Parameters<APIRoute>[0]) {
  return requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await guardOf(ctx);
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const { live, data } = await loadMigrations(guard.session);
  return json({ live, migrations: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await guardOf(ctx);
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const input = sanitizeMigrationInput(await ctx.request.json().catch(() => null));
  if (!input) {
    return json(
      {
        error: 'bad_request',
        message: 'Faltan datos: la conexión de origen, qué traer y para quién.',
      },
      400,
    );
  }
  try {
    return json({ migrations: await createMigrations(guard.session, input) }, 202);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
