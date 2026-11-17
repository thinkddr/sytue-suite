/* SPDX-License-Identifier: Apache-2.0 */
import { rutaMigracionMatrixPrivada } from '@sytue/auth/server';
import { bff } from '../../../../../lib/auth.js';
export const prerender = false;
export const GET = rutaMigracionMatrixPrivada({ bff, env: import.meta.env });
export const POST = GET;
