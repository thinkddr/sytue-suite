/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Admin — integraciones de serie (nubes, chats y videollamadas).
 *
 *   GET /api/admin/apps/integrations                    Cuáles están encendidas.
 *   PUT /api/admin/apps/integrations?provider=…  body { enabled }
 *
 * Cada persona las conecta desde la app con solo el consentimiento; aquí solo se
 * apagan. Passthrough al gateway, solo admin/owner.
 */
import type { APIRoute } from 'astro';
import { requireSession, type GuardOptions } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  loadBuiltinIntegrations,
  setBuiltinIntegration,
  proxyStatus,
} from '../../../../lib/gateway.js';

export const prerender = false;

const ADMIN: GuardOptions = { roles: ['admin', 'owner'], perms: ['apps.manage'] };

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), ADMIN);
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const res = await loadBuiltinIntegrations(guard.session);
  return json({ live: res.live, items: res.data });
};

export const PUT: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), ADMIN);
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const provider = new URL(ctx.request.url).searchParams.get('provider')?.trim() ?? '';
  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  if (!provider || typeof body?.enabled !== 'boolean') return json({ error: 'bad_request' }, 400);
  try {
    return json(await setBuiltinIntegration(guard.session, provider, body.enabled));
  } catch (e) {
    return json({ error: 'gateway_error' }, proxyStatus(e));
  }
};
