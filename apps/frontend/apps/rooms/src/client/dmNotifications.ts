/* SPDX-License-Identifier: Apache-2.0 */
import type { DmNotificationAPI, NotificationOwner } from '../lib/dmNotifications.js';

/** The BFF pins each operation to the identity that opened this screen. */
export function dmNotificationAPI(owner: NotificationOwner): DmNotificationAPI {
  async function request(
    method: 'GET' | 'PUT',
    signal: AbortSignal,
    enabled?: boolean,
  ): Promise<Response> {
    const response = await fetch('/api/dm-notifications', {
      method,
      credentials: 'include',
      cache: 'no-store',
      signal,
      headers: {
        accept: 'application/json',
        'x-sytue-preference-owner': encodeURIComponent(owner.id),
        'x-sytue-preference-tenant': encodeURIComponent(owner.tenant),
        ...(method === 'PUT' ? { 'content-type': 'application/json' } : {}),
      },
      ...(method === 'PUT' ? { body: JSON.stringify({ enabled }) } : {}),
    });
    if (!response.ok) throw new Error('notification_preference');
    return response;
  }
  return {
    async read(signal) {
      const result = (await (await request('GET', signal)).json()) as { enabled?: unknown };
      if (typeof result.enabled !== 'boolean') throw new Error('notification_preference');
      return result.enabled;
    },
    async write(enabled, signal) {
      await request('PUT', signal, enabled);
    },
  };
}
