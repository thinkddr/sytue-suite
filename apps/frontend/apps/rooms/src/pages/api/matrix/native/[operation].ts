/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { chatSdk } from '../../../../lib/chat.js';
import { getApiSession } from '../../../../lib/session.js';
import { opaqueMatrixNative } from '../../../../client/matrix/native.js';
export const prerender = false;
const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
      'referrer-policy': 'no-referrer',
    },
  });
export const GET: APIRoute = async (ctx) => {
  if (ctx.params.operation !== 'preview') return json({ error: 'not_found' }, 404);
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const state = ctx.url.searchParams.get('state');
  if (!opaqueMatrixNative(state)) return json({ error: 'invalid_input' }, 400);
  try {
    return json(await chatSdk(session.accessToken).previewMatrixNative(state));
  } catch (error) {
    return json({ error: 'matrix_unavailable' }, upstreamStatus(error));
  }
};
export const POST: APIRoute = async (ctx) => {
  if (!['start', 'accept', 'finish'].includes(ctx.params.operation ?? ''))
    return json({ error: 'not_found' }, 404);
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  if (Number(ctx.request.headers.get('content-length')) > 16384)
    return json({ error: 'invalid_input' }, 413);
  const input = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  if (!input) return json({ error: 'invalid_input' }, 400);
  try {
    const sdk = chatSdk(session.accessToken);
    if (ctx.params.operation === 'start') {
      if (!opaqueMatrixNative(input.code_challenge) || input.code_challenge_method !== 'S256')
        return json({ error: 'invalid_input' }, 400);
      return json(
        await sdk.startMatrixNative({
          code_challenge: input.code_challenge,
          code_challenge_method: 'S256',
        }),
        201,
      );
    }
    if (!opaqueMatrixNative(input.state)) return json({ error: 'invalid_input' }, 400);
    if (ctx.params.operation === 'accept') {
      if (
        typeof input.login_token !== 'string' ||
        input.login_token.length < 16 ||
        input.login_token.length > 8192 ||
        /[\r\n\0]/.test(input.login_token)
      )
        return json({ error: 'invalid_input' }, 400);
      return json(
        await sdk.acceptMatrixNative({ state: input.state, login_token: input.login_token }),
        201,
      );
    }
    if (
      !opaqueMatrixNative(input.ticket) ||
      typeof input.code_verifier !== 'string' ||
      !/^[A-Za-z0-9._~-]{43,128}$/.test(input.code_verifier)
    )
      return json({ error: 'invalid_input' }, 400);
    return json(
      await sdk.finishMatrixNative({
        state: input.state,
        ticket: input.ticket,
        code_verifier: input.code_verifier,
      }),
    );
  } catch (error) {
    return json({ error: 'matrix_unavailable' }, upstreamStatus(error));
  }
};
