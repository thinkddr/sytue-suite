/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/ai  (application/json) — el asistente de Sytue Notepads.
 *
 * El proxy entero está en `@sytue/ai/astro`: validación del cuerpo, topes de
 * tamaño, los dos plazos de espera y el reenvío firmado con la sesión. Era el
 * MISMO fichero en cuatro apps. Aquí solo queda lo que es de esta: qué entorno
 * lee y de dónde saca la sesión.
 */
import { crearProxyDeAsistente } from '@sytue/ai/astro';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';

export const prerender = false;

export const { GET, POST } = crearProxyDeAsistente(serverEnv(import.meta.env), getApiSession);
