/**
 * Actividad reciente de la propia cuenta: inicios de sesión y cambios de seguridad.
 * Proxy hacia `GET /api/v1/me/activity` de Sytue ID por el BFF, para que el access
 * token no salga al navegador.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => proxyId(ctx, '/api/v1/me/activity');
