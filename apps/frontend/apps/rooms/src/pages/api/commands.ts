/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Rooms — SLASH COMMANDS de terceros.
 *
 *   GET  /api/commands                                   → los registrados en la org
 *   POST /api/commands { action:'execute', channelId, input }  → ejecuta y devuelve
 *   POST /api/commands { action:'create', … }            → registra (admin)
 *   POST /api/commands { action:'delete', id }           → borra (admin)
 *
 * El secreto de firma solo baja para un admin: el servicio Go lo recorta según quién
 * pregunta, y aquí no se toca. Las mutaciones van por POST con cuerpo JSON porque el
 * checkOrigin de Astro rechaza DELETE tras el proxy.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    return json({ commands: await client.listCommands() });
  } catch {
    // Que la lista falle no debe romper el compositor: sin comandos de servidor, los
    // integrados siguen funcionando.
    return json({ commands: [] });
  }
};

interface Payload {
  action?: string;
  id?: string;
  channelId?: string;
  input?: string;
  name?: string;
  description?: string;
  usageHint?: string;
  requestUrl?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let p: Payload;
  try {
    p = (await ctx.request.json()) as Payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    switch (p.action) {
      case 'execute': {
        if (!p.channelId || !p.input?.trim()) return json({ error: 'missing_fields' }, 400);
        return json(await client.executeCommand(p.channelId, p.input));
      }
      case 'create': {
        if (!p.name?.trim() || !p.requestUrl?.trim()) return json({ error: 'missing_fields' }, 400);
        return json(
          await client.createCommand({
            name: p.name.trim(),
            requestUrl: p.requestUrl.trim(),
            ...(p.description?.trim() ? { description: p.description.trim() } : {}),
            ...(p.usageHint?.trim() ? { usageHint: p.usageHint.trim() } : {}),
          }),
          201,
        );
      }
      case 'delete': {
        if (!p.id) return json({ error: 'missing_id' }, 400);
        await client.deleteCommand(p.id);
        return json({ ok: true });
      }
      default:
        return json({ error: 'unknown_action' }, 400);
    }
  } catch (err) {
    // El código importa: 404 = no existe el comando, 502 = existe pero no contestó.
    // Colapsarlos en un 500 dejaría a la persona sin saber cuál de las dos cosas pasó.
    const status =
      typeof (err as { status?: number }).status === 'number'
        ? (err as { status: number }).status
        : 502;
    return json({ error: 'command_failed' }, status);
  }
};
