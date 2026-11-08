/* SPDX-License-Identifier: Apache-2.0 */
// La clave de recuperación del cifrado, custodiada por el servicio para la persona
// de la sesión OIDC (ADR 0081). El BFF no la guarda ni la registra: la pasa.
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { getApiSession } from '../../../lib/session.js';
import { chatSdk } from '../../../lib/chat.js';
import { recoveryKeyShape } from '../../../client/matrix/recovery.js';
export const prerender = false;
const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const { recovery_key } = await chatSdk(session.accessToken).matrixRecoveryKey(
      ctx.request.signal,
    );
    if (!recoveryKeyShape.test(recovery_key)) return json({ error: 'matrix_unavailable' }, 502);
    return json({ recovery_key });
  } catch (error) {
    return json({ error: 'matrix_unavailable' }, upstreamStatus(error));
  }
};
export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  if (Number(ctx.request.headers.get('content-length')) > 1024)
    return json({ error: 'invalid_input' }, 413);
  const input = (await ctx.request.json().catch(() => null)) as { recovery_key?: unknown } | null;
  if (typeof input?.recovery_key !== 'string' || !recoveryKeyShape.test(input.recovery_key))
    return json({ error: 'invalid_input' }, 400);
  try {
    await chatSdk(session.accessToken).storeMatrixRecoveryKey(input.recovery_key);
    return new Response(null, { status: 204, headers: { 'cache-control': 'no-store' } });
  } catch (error) {
    return json({ error: 'matrix_unavailable' }, upstreamStatus(error));
  }
};
