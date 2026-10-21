/** Empieza el alta del segundo factor: secreto, URI otpauth y su QR ya pintado. */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) =>
  proxyId(ctx, '/api/v1/me/mfa/totp/enroll', { method: 'POST' });
