/**
 * La firma que la persona pidió recordar en CertiPDF — proxy BFF hacia Sytue ID.
 *
 *   GET    /api/rubrica -> GET    /api/v1/me/rubrica  {trazos, updated_at} | 404
 *   DELETE /api/rubrica -> DELETE /api/v1/me/rubrica
 *
 * Aquí solo se ve y se borra: guardarla es cosa de CertiPDF, al firmar.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = (ctx) => proxyId(ctx, '/api/v1/me/rubrica');

export const DELETE: APIRoute = (ctx) => proxyId(ctx, '/api/v1/me/rubrica', { method: 'DELETE' });
