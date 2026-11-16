/* SPDX-License-Identifier: Apache-2.0 */
import { rutaCorteIdentidadMatrixPrivada } from '@sytue/auth/server';
import { bff } from '@sytue/office/auth';
export const prerender = false;
export const GET = rutaCorteIdentidadMatrixPrivada({
  bff,
  env: import.meta.env,
  product: 'sheets',
});
export const POST = GET;
