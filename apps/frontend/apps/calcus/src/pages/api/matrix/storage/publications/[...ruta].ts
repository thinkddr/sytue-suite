// SPDX-License-Identifier: Apache-2.0
import { rutaPublicacionesMatrixPrivadas } from '@sytue/auth/server';
import { bff } from '@sytue/office/auth';
export const GET = rutaPublicacionesMatrixPrivadas({
  bff,
  env: import.meta.env,
  product: 'sheets',
});
export const POST = GET;
