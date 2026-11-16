/* SPDX-License-Identifier: Apache-2.0 */
import { rutaMigracionMatrixPrivada } from '@sytue/auth/server';
import { bff } from '../../../../lib/bff.js';
export const prerender = false;
export const GET = rutaMigracionMatrixPrivada({ bff, env: import.meta.env, product: 'calendar' });
export const POST = GET;
