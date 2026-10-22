/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — enlaces de compartir.
 *
 *   POST   /api/shares            { path, permission?: 'view'|'edit', password?, expiresAt? }
 *                                  { path, email, permission? } → comparte con esa PERSONA
 *                                  { path, group, permission? } → comparte con ese GRUPO
 *   GET    /api/shares?path=/ruta Lista los enlaces vivos de un recurso.
 *   DELETE /api/shares?id=…       Revoca un enlace.
 *
 * Proxy fino sobre DriveClient (@sytue/api-clients) → servicio Go propio
 * (drive.sytue.eu) con el access token de la sesion httpOnly. El token NUNCA
 * llega al cliente.
 */
import type { APIRoute } from 'astro';
import { HttpError, upstreamStatus, type CreateShareInput } from '@sytue/api-clients';
import { driveClientFor } from '../../lib/auth';
import { resolverPersona, resolverPersonas } from '../../lib/directorio';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  let body: {
    path?: unknown;
    permission?: unknown;
    password?: unknown;
    expiresAt?: unknown;
    email?: unknown;
    group?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const path = typeof body.path === 'string' ? body.path.trim() : '';
  if (!path) return json({ error: 'missing_path' }, 400);

  const input: CreateShareInput = {
    path,
    permission: body.permission === 'edit' ? 'edit' : 'view',
  };

  // Compartir con una PERSONA: se escribe un correo y por dentro se guarda su
  // identificador. La traducción la hace el servicio de identidad y NO el
  // navegador: un `subject` que eligiera el cliente sería conceder un permiso a
  // quien se quisiera.
  const email = typeof body.email === 'string' ? body.email.trim() : '';
  // El GRUPO no se resuelve contra Sytue ID: su nombre YA ES el identificador que
  // Drive matchea (ver pages/api/groups.ts), a diferencia de una persona, cuyo
  // correo hay que traducir a un `sub` estable.
  const group = typeof body.group === 'string' ? body.group.trim() : '';
  if (email) {
    const persona = await resolverPersona(ctx, email);
    if (!persona) return json({ error: 'unknown_person' }, 404);
    input.subject = persona.userId;
  } else if (group) {
    input.subject = group;
    input.subjectKind = 'group';
  } else {
    if (typeof body.password === 'string' && body.password) input.password = body.password;
    if (typeof body.expiresAt === 'string' && body.expiresAt) input.expiresAt = body.expiresAt;
  }

  try {
    const share = await client.createShare(input);
    return json({ share }, 201);
  } catch (e) {
    const status = upstreamStatus(e);
    // El servicio puede negar el enlace por una ETIQUETA DE SENSIBILIDAD del archivo
    // (403). Ese «por qué» se propaga tal cual para que la UI lo diga en vez de un
    // «no se pudo» seco. Solo se propaga el mensaje de los 4xx del servicio.
    return json({ error: 'share_failed', message: upstreamMessage(e) }, status);
  }
};

/**
 * Mensaje legible que devolvió el servicio Go (sobre de error httpx), si lo hay.
 * Cadena vacía cuando no se puede leer: la UI cae a su texto genérico.
 */
function upstreamMessage(e: unknown): string {
  if (!(e instanceof HttpError) || !e.body) return '';
  try {
    const parsed = JSON.parse(e.body) as { error?: { message?: unknown } };
    const msg = parsed?.error?.message;
    return typeof msg === 'string' ? msg : '';
  } catch {
    return '';
  }
}

export const GET: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const path = new URL(ctx.request.url).searchParams.get('path');
  if (!path) return json({ error: 'missing_path' }, 400);

  try {
    const shares = await client.listShares(path);
    // Las comparticiones con personas guardan un identificador; aquí se le pone
    // cara. Sin esto la lista diría «compartido con 8f3a-…», que no le sirve a
    // nadie para decidir si eso sigue teniendo sentido.
    const personas = await resolverPersonas(
      ctx,
      shares.filter((s) => s.kind === 'user').map((s) => s.subject),
    );
    return json({
      shares: shares.map((s) => {
        // Un grupo no necesita resolverse: `subject` YA ES su nombre legible
        // (ver pages/api/groups.ts) — a diferencia de una persona, cuyo `subject`
        // es un `sub` opaco.
        if (s.kind === 'group') return { ...s, name: s.subject };
        const p = personas.get(s.subject);
        return p
          ? { ...s, ...(p.email ? { email: p.email } : {}), ...(p.name ? { name: p.name } : {}) }
          : s;
      }),
    });
  } catch (e) {
    return json({ error: 'list_failed' }, upstreamStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const id = new URL(ctx.request.url).searchParams.get('id');
  if (!id) return json({ error: 'missing_id' }, 400);

  try {
    await client.revokeShare(id);
    return json({ ok: true });
  } catch (e) {
    return json({ error: 'revoke_failed' }, upstreamStatus(e));
  }
};
