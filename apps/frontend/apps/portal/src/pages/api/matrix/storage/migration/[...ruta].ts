/* SPDX-License-Identifier: Apache-2.0 */
import { rutaCorteIdentidadMatrixPrivada } from '@sytue/auth/server';
import { bff } from '../../../../../lib/auth.js';
export const prerender = false;
export const GET = rutaCorteIdentidadMatrixPrivada({
  bff,
  env: import.meta.env,
  product: 'hub',
});
export const POST = GET;
