/**
 * Sesiones del propio usuario: listar y cerrar todas menos la actual.
 * Proxy hacia `/api/v1/me/sessions` de Sytue ID por el BFF, para que el access
 * token no salga al navegador.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => proxyId(ctx, '/api/v1/me/sessions');

export const DELETE: APIRoute = async (ctx) =>
  proxyId(ctx, '/api/v1/me/sessions', { method: 'DELETE' });
