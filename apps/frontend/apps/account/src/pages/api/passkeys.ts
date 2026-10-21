/** Las passkeys registradas. El alta vive en el navegador (WebAuthn); aquí solo se listan. */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => proxyId(ctx, '/api/v1/webauthn/credentials');
