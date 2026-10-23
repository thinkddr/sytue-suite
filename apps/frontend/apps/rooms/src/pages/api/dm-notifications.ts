/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { HubClient, bearer } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { dmNotificationResponse } from '../../lib/dmNotificationsServer.js';

export const prerender = false;
const handler: APIRoute = async (context) => {
  const session = await getApiSession(context);
  const hub = new HubClient({
    baseUrl: serverEnv(import.meta.env).SYTUE_HUB_BASE_URL ?? 'https://hub.sytue.eu',
    auth: bearer(session?.accessToken ?? ''),
  });
  return dmNotificationResponse(context.request, session, hub);
};
export const GET = handler;
export const PUT = handler;
