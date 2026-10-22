/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { isMatrixDeviceId } from '../../client/matrix/types.js';
import { getApiSession } from '../../lib/session.js';
import { meetClient, MeetError } from '../../lib/meet.js';
export const prerender = false;
const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
function inputs(room: unknown, device: unknown, join: unknown): room is string {
  const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
  return (
    typeof room === 'string' &&
    uuid.test(room) &&
    isMatrixDeviceId(device) &&
    typeof join === 'string' &&
    uuid.test(join)
  );
}
export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const room = ctx.url.searchParams.get('roomId'),
    device = ctx.url.searchParams.get('matrixDeviceId'),
    join = ctx.url.searchParams.get('joinId');
  if (!inputs(room, device, join)) return json({ error: 'invalid_input' }, 400);
  try {
    return json(
      await meetClient({ accessToken: session.accessToken }).encryption(room, device!, join!),
    );
  } catch (error) {
    return json(
      { error: error instanceof MeetError ? error.code : 'encryption_unavailable' },
      error instanceof MeetError ? error.status : 502,
    );
  }
};
export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  if (
    !body ||
    !inputs(body.roomId, body.matrixDeviceId, body.joinId) ||
    !Number.isSafeInteger(body.generation) ||
    Number(body.generation) < 1 ||
    !['ready', 'reconnect'].includes(String(body.action))
  )
    return json({ error: 'invalid_input' }, 400);
  const room = body.roomId,
    device = body.matrixDeviceId as string,
    join = body.joinId as string,
    generation = body.generation as number;
  try {
    const sdk = meetClient({ accessToken: session.accessToken });
    if (body.action === 'reconnect')
      return json(await sdk.reconnectEncryption(room, device, join, generation));
    await sdk.readyEncryption(room, device, join, generation);
    return json({});
  } catch (error) {
    return json(
      { error: error instanceof MeetError ? error.code : 'encryption_unavailable' },
      error instanceof MeetError ? error.status : 502,
    );
  }
};
