/**
 * Cierra el alta: verifica la attestation y registra la passkey.
 *
 * El cuerpo se reenvía tal cual porque lo compone el navegador a partir de lo que
 * devuelve el autenticador: recortarlo aquí por una lista blanca sería adivinar
 * qué campos necesita la verificación, y el servicio ya los valida.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  let cuerpo: string;
  try {
    cuerpo = JSON.stringify(await ctx.request.json());
  } catch {
    return new Response(null, { status: 400 });
  }
  return proxyId(ctx, '/api/v1/webauthn/register/finish', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: cuerpo,
  });
};
