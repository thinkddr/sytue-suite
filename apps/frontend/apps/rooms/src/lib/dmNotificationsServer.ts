/* SPDX-License-Identifier: Apache-2.0 */
import type { HubClient } from '@sytue/api-clients';
import type { ServerSession } from '@sytue/auth/server';
import {
  DM_NOTIFICATION_RULE,
  dmNotificationsEnabled,
  notificationOwnerMatches,
} from './dmNotifications.js';

function json(body: unknown, status = 200): Response {
  return Response.json(body, { status, headers: { 'cache-control': 'no-store' } });
}

/** The expected identity is only a stale-screen guard; the authenticated session authorizes. */
export async function dmNotificationResponse(
  request: Request,
  session: ServerSession | null,
  hub: Pick<HubClient, 'notificationPrefs' | 'setNotificationPref'>,
): Promise<Response> {
  if (!session) return json({ error: 'unauthenticated' }, 401);
  let expected;
  try {
    expected = {
      id: decodeURIComponent(request.headers.get('x-sytue-preference-owner') ?? ''),
      tenant: decodeURIComponent(request.headers.get('x-sytue-preference-tenant') ?? ''),
    };
  } catch {
    return json({ error: 'session_changed' }, 409);
  }
  if (
    !notificationOwnerMatches(expected, { id: session.user.id, tenant: session.user.tenant ?? '' })
  ) {
    return json({ error: 'session_changed' }, 409);
  }
  if (request.method === 'GET') {
    try {
      return json({ enabled: dmNotificationsEnabled(await hub.notificationPrefs()) });
    } catch {
      return json({ error: 'hub_unavailable' }, 502);
    }
  }
  if (request.method !== 'PUT') return json({ error: 'method_not_allowed' }, 405);
  // A foreign origin cannot turn a saved browser setting into an account write.
  if (request.headers.get('origin') !== new URL(request.url).origin) {
    return json({ error: 'invalid_origin' }, 403);
  }
  let payload: unknown;
  try {
    payload = await request.json();
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  if (
    !payload ||
    typeof payload !== 'object' ||
    !('enabled' in payload) ||
    typeof payload.enabled !== 'boolean'
  ) {
    return json({ error: 'invalid_preference' }, 400);
  }
  try {
    // Update exactly one rule; an unrelated mute is never replaced by a stale list.
    await hub.setNotificationPref({ ...DM_NOTIFICATION_RULE, muted: !payload.enabled });
    return json({ ok: true });
  } catch {
    return json({ error: 'hub_unavailable' }, 502);
  }
}
