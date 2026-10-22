/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import {
  whatsAppSignupStatus,
  startWhatsAppSignup,
  finishWhatsAppSignup,
  listWhatsAppNumbers,
  upstreamStatus,
} from '@sytue/api-clients';
import type { WhatsAppSignupFinish } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { appConnectionsClient } from '../../lib/app-connections.js';
import { chatSdk } from '../../lib/chat.js';

export const prerender = false;
const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  if (!session.user.roles.some((role) => role === 'owner' || role === 'admin'))
    return json({ available: false }, 403);
  try {
    const client = appConnectionsClient(session.accessToken);
    const instance = ctx.url.searchParams.get('instance');
    return json(
      instance ? await listWhatsAppNumbers(client, instance) : await whatsAppSignupStatus(client),
    );
  } catch (error) {
    return json({ error: 'whatsapp_unavailable' }, upstreamStatus(error));
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  if (!session.user.roles.some((role) => role === 'owner' || role === 'admin'))
    return json({ error: 'forbidden' }, 403);
  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  if (!body || typeof body !== 'object') return json({ error: 'invalid_json' }, 400);
  const value = (key: string): string => (typeof body[key] === 'string' ? body[key] : '');
  try {
    const client = appConnectionsClient(session.accessToken);
    if (body.action === 'start')
      return json(await startWhatsAppSignup(client, value('instance_id') || undefined));
    if (body.action === 'finish') {
      const flow = value('flow');
      if (
        !value('state') ||
        !value('code') ||
        (flow &&
          !['FINISH', 'FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING', 'FINISH_ONLY_WABA'].includes(flow))
      )
        return json({ error: 'missing_fields' }, 400);
      return json(
        await finishWhatsAppSignup(client, {
          state: value('state'),
          code: value('code'),
          ...(value('waba_id') ? { waba_id: value('waba_id') } : {}),
          ...(value('phone_number_id') ? { phone_number_id: value('phone_number_id') } : {}),
          ...(flow ? { flow: flow as NonNullable<WhatsAppSignupFinish['flow']> } : {}),
        }),
      );
    }
    if (body.action === 'connect') {
      const team = value('team_id'),
        instance = value('instance_id'),
        phone = value('phone_number_id'),
        pin = value('pin');
      if (!team || !instance || !phone || (pin && !/^\d{6}$/.test(pin)))
        return json({ error: 'missing_fields' }, 400);
      return json(
        await chatSdk(session.accessToken).connectWhatsAppConsent({
          team_id: team,
          instance_id: instance,
          phone_number_id: phone,
          ...(pin ? { pin } : {}),
        }),
        201,
      );
    }
    return json({ error: 'invalid_action' }, 400);
  } catch (error) {
    return json({ error: 'whatsapp_connection_failed' }, upstreamStatus(error));
  }
};
