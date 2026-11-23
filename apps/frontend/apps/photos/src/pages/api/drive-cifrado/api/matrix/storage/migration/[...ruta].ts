/* SPDX-License-Identifier: Apache-2.0 */
/*
 * El estado de la fuente del Drive del servidor, visto desde Photos. Solo GET: la
 * galería lo lee para saber si el Drive de la persona está en la bóveda y abrirla
 * con su recibo; nunca empieza una migración (eso lo hace Drive).
 */
import { rutaMigracionMatrixPrivada } from '@sytue/auth/server';
import { bff } from '../../../../../../../lib/bff.js';
export const prerender = false;
export const GET = rutaMigracionMatrixPrivada({ bff, env: import.meta.env, product: 'drive' });
