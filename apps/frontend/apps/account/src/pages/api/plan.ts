/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/plan — el contrato de la organización activa, tal como lo cuenta
 * Sytue ID (plan vigente, estado del pago y cuota que concede). Proxy con la
 * cookie de sesión: el access token no sale del servidor.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => proxyId(ctx, '/api/v1/onboarding/subscription');
