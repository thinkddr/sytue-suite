/** Confirma el alta con el primer código. Devuelve los códigos de recuperación UNA vez. */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  let code = '';
  try {
    const body = (await ctx.request.json()) as { code?: unknown };
    code = typeof body.code === 'string' ? body.code.trim() : '';
  } catch {
    return new Response(null, { status: 400 });
  }
  if (!code) return new Response(null, { status: 400 });
  return proxyId(ctx, '/api/v1/me/mfa/totp/confirm', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ code }),
  });
};
