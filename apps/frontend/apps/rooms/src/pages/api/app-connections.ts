/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import {
  isRoomsProvider,
  HttpError,
  disconnectRoomsConnection,
  listRoomsConnections,
  ROOMS_PROVIDERS,
  startAppConsent,
  upstreamStatus,
} from '@sytue/api-clients';
import { getApiSession } from '../../lib/session';
import { appConnectionsClient } from '../../lib/app-connections';
import { chatSdk } from '../../lib/chat';

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
  if (!session.user.roles.some((r) => r === 'owner' || r === 'admin')) {
    return json({ available: false, providers: [], items: [] }, 403);
  }
  try {
    const instance = ctx.url.searchParams.get('instance');
    return json(
      instance
        ? await chatSdk(session.accessToken).integrationDestinations(instance)
        : await listRoomsConnections(appConnectionsClient(session.accessToken)),
    );
  } catch (error) {
    return json({ error: 'connections_unavailable' }, upstreamStatus(error));
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  if (!session.user.roles.some((r) => r === 'owner' || r === 'admin')) {
    return json({ error: 'forbidden' }, 403);
  }
  const body = (await ctx.request.json().catch(() => null)) as {
    action?: unknown;
    provider?: unknown;
    instance_id?: unknown;
    new_account?: unknown;
    channel_id?: unknown;
    remote_id?: unknown;
  } | null;
  if (!body || typeof body !== 'object') return json({ error: 'invalid_json' }, 400);
  const instance = typeof body.instance_id === 'string' ? body.instance_id : '';
  const channel = typeof body.channel_id === 'string' ? body.channel_id : '';
  try {
    if (body.action === 'consent' || (!body.action && body.provider)) {
      if (!isRoomsProvider(body.provider)) return json({ error: 'invalid_provider' }, 400);
      if (body.new_account === true && instance) return json({ error: 'invalid_account' }, 400);
      return json(
        await startAppConsent(
          appConnectionsClient(session.accessToken),
          ROOMS_PROVIDERS[body.provider],
          'rooms',
          instance || undefined,
          { newAccount: body.new_account === true },
        ),
      );
    }
    if (body.action === 'disconnect') {
      if (!instance) return json({ error: 'missing_fields' }, 400);
      await disconnectRoomsConnection(appConnectionsClient(session.accessToken), instance);
      return json({ ok: true });
    }
    if (!instance || !channel) return json({ error: 'missing_fields' }, 400);
    // The Rooms service verifies local channel administration before calling providers.
    const rooms = chatSdk(session.accessToken);
    if (body.action === 'connect-bridge' && typeof body.remote_id === 'string' && body.remote_id) {
      const connections = await listRoomsConnections(appConnectionsClient(session.accessToken));
      const account = connections.items.find((item) => item.id === instance && item.connected);
      const platform =
        account?.template_key === 'slack-rooms'
          ? 'slack'
          : account?.template_key === 'microsoft-teams-rooms'
            ? 'teams'
            : account?.template_key === 'google-chat'
              ? 'googlechat'
              : null;
      if (!platform) return json({ error: 'invalid_provider' }, 400);
      return json(await rooms.connectOAuthBridge(channel, instance, body.remote_id, platform), 201);
    }
    return json({ error: 'invalid_action' }, 400);
  } catch (error) {
    if (error instanceof HttpError && error.code === 'oauth_provider_unavailable') {
      return json({ error: 'oauth_provider_unavailable' }, 503);
    }
    return json({ error: 'connection_failed' }, upstreamStatus(error));
  }
};
