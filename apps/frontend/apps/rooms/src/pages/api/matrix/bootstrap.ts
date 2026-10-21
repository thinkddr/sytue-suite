/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { getApiSession } from '../../../lib/session.js';
import { chatSdk } from '../../../lib/chat.js';
export const prerender = false;
export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  const headers = {
    'content-type': 'application/json; charset=utf-8',
    'cache-control': 'no-store',
  };
  if (!session)
    return new Response(JSON.stringify({ error: 'unauthenticated' }), { status: 401, headers });
  try {
    const data = await chatSdk(session.accessToken).matrixBootstrap(ctx.request.signal);
    return new Response(JSON.stringify(data), { headers });
  } catch (error) {
    return new Response(JSON.stringify({ error: 'matrix_unavailable' }), {
      status: upstreamStatus(error),
      headers,
    });
  }
};
