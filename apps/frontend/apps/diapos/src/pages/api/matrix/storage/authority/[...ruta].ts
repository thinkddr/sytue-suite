// SPDX-License-Identifier: Apache-2.0
import { rutaAutoridadMatrixPrivada } from '@sytue/auth/server';
import { bff } from '@sytue/office/auth';
export const GET = rutaAutoridadMatrixPrivada({ bff, env: import.meta.env, product: 'slides' });
export const POST = GET;
