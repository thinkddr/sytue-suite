/* SPDX-License-Identifier: Apache-2.0 */
// La bóveda post-cuántica del correo (ADR 0087, entrega 4). Detrás del conmutador
// de despliegue: con SYTUE_MAIL_BOVEDA distinto de `activado` la ruta no existe.
import type { APIRoute } from 'astro';
import { rutaBovedaPq, serverEnv } from '@sytue/auth/server';
import { bff } from '../../../lib/bff.js';
import { correoSobreBoveda } from '../../../client/almacenCorreo.js';
export const prerender = false;
const boveda = rutaBovedaPq({ bff, env: import.meta.env, product: 'mail' });
export const GET: APIRoute = (ctx) =>
  correoSobreBoveda(serverEnv(import.meta.env).SYTUE_MAIL_BOVEDA)
    ? boveda(ctx)
    : new Response(null, { status: 404, headers: { 'cache-control': 'no-store' } });
export const PUT = GET;
export const POST = GET;
