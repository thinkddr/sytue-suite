/** Genera un lote nuevo de códigos de recuperación e invalida el anterior. */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) =>
  proxyId(ctx, '/api/v1/me/mfa/totp/recovery-codes', { method: 'POST' });
