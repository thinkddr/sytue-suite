/* SPDX-License-Identifier: Apache-2.0 */
import { rutaCorteIdentidadMatrixPrivada } from '@sytue/auth/server';
import { bff } from '../../../../../lib/bff.js';
export const prerender = false;
export const GET = rutaCorteIdentidadMatrixPrivada({
  bff,
  env: import.meta.env,
  product: 'bases',
});
export const POST = GET;
