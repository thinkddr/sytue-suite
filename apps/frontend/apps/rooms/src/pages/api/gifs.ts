/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/** Catálogo CC0: el navegador solo contacta con Rooms; no envía IP/cookies a Commons. */
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { chatSdk } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return Response.json({ error: 'unauthenticated' }, { status: 401 });
  let input: { query?: unknown; cursor?: unknown };
  try {
    input = (await ctx.request.json()) as typeof input;
  } catch {
    return Response.json({ error: 'invalid_json' }, { status: 400 });
  }
  if (
    !input ||
    typeof input !== 'object' ||
    Array.isArray(input) ||
    typeof input.query !== 'string' ||
    (input.cursor !== undefined && typeof input.cursor !== 'string')
  )
    return Response.json({ error: 'invalid_query' }, { status: 400 });
  try {
    return Response.json(
      await chatSdk(session.accessToken).searchFreeGifs(
        input.query,
        input.cursor,
        ctx.request.signal,
      ),
      {
        headers: { 'cache-control': 'no-store' },
      },
    );
  } catch (err) {
    return Response.json({ error: 'gif_catalog_unavailable' }, { status: upstreamStatus(err) });
  }
};

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response('unauthenticated', { status: 401 });
  const id = ctx.url.searchParams.get('id') ?? '';
  if (!/^[1-9][0-9]{0,14}$/.test(id)) return new Response('invalid_id', { status: 400 });
  const preview = ctx.url.searchParams.get('preview') === '1';
  try {
    const result = await chatSdk(session.accessToken).freeGifContent(
      Number(id),
      preview,
      ctx.request.signal,
    );
    return new Response(result.body, {
      headers: {
        'content-type': preview ? 'image/png' : 'image/gif',
        'cache-control': 'private, max-age=300',
        'x-content-type-options': 'nosniff',
        'content-security-policy': 'sandbox',
      },
    });
  } catch (err) {
    return new Response('gif_unavailable', { status: upstreamStatus(err) });
  }
};
