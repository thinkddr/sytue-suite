// SPDX-License-Identifier: Apache-2.0
import { rutaPublicacionesMatrixPrivadas } from '@sytue/auth/server';
import { bff } from '../../../../../lib/bff.js';
export const GET = rutaPublicacionesMatrixPrivadas({
  bff,
  env: import.meta.env,
  product: 'calendar',
});
export const POST = GET;
