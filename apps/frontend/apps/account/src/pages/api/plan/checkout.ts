/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/plan/checkout — inicia el cobro del plan de 1 TB en Sytue ID, que
 * decide el contrato y el importe (una cuenta personal no elige asientos: los
 * que mande el navegador se ignoran allí). Devuelve la URL de pago de Mollie.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) =>
  proxyId(ctx, '/api/v1/onboarding/subscription/checkout', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ seats: 1 }),
  });
