/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';
export const prerender = false;
const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', 'cache-control': 'no-store' },
  });
export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  for (const key of ctx.url.searchParams.keys())
    if (key !== 'cursor') return json({ error: 'invalid_metadata_request' }, 400);
  try {
    return json(
      await chatClient({
        accessToken: session.accessToken,
        userId: session.user.id,
      }).messageMetadata(ctx.url.searchParams.get('cursor') ?? '', ctx.request.signal),
    );
  } catch (error) {
    return json({ error: 'chat_error' }, upstreamStatus(error));
  }
};
export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  let input: unknown;
  try {
    input = await ctx.request.json();
  } catch {
    return json({ error: 'invalid_metadata_request' }, 400);
  }
  if (
    !input ||
    typeof input !== 'object' ||
    Object.keys(input).some((k) => k !== 'messages') ||
    !('messages' in input) ||
    !Array.isArray(input.messages) ||
    input.messages.length === 0 ||
    input.messages.length > 200 ||
    input.messages.some(
      (m: unknown) =>
        !m ||
        typeof m !== 'object' ||
        !('id' in m) ||
        !('tenant' in m) ||
        typeof m.id !== 'string' ||
        typeof m.tenant !== 'string' ||
        Object.keys(m).some((k) => k !== 'id' && k !== 'tenant'),
    )
  )
    return json({ error: 'invalid_metadata_request' }, 400);
  try {
    return json({
      messages: await chatClient({
        accessToken: session.accessToken,
        userId: session.user.id,
      }).messageMetadataVisible(
        input.messages as { id: string; tenant: string }[],
        ctx.request.signal,
      ),
    });
  } catch (error) {
    return json({ error: 'chat_error' }, upstreamStatus(error));
  }
};
