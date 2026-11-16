/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
export const prerender = false;
// Old clients must update instead of forwarding private search terms to Rooms.
const localSearchRequired: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  return new Response(
    JSON.stringify({ error: session ? 'local_search_required' : 'unauthenticated' }),
    {
      status: session ? 410 : 401,
      headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
    },
  );
};
export const GET: APIRoute = localSearchRequired;
export const QUERY: APIRoute = localSearchRequired;
