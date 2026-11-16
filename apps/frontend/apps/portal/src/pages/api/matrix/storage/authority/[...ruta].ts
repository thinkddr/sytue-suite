// SPDX-License-Identifier: Apache-2.0
import { rutaAutoridadMatrixPrivada } from '@sytue/auth/server';
import { bff } from '../../../../../lib/auth.js';
export const GET = rutaAutoridadMatrixPrivada({ bff, env: import.meta.env, product: 'hub' });
export const POST = GET;
