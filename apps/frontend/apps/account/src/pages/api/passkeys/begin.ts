/**
 * Empieza el alta de una passkey: devuelve las opciones que espera
 * `navigator.credentials.create()`, con el reto ya persistido en el servicio.
 *
 * NO es un candidato a QUERY aunque «no devuelva datos que cambien»: acuña y
 * guarda un reto, así que escribe. Un método seguro anunciaría lo contrario y
 * los intermediarios podrían reintentarlo.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) =>
  proxyId(ctx, '/api/v1/webauthn/register/begin', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: '{}',
  });
