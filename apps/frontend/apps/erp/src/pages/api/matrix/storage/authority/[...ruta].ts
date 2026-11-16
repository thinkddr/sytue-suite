// SPDX-License-Identifier: Apache-2.0
import { rutaAutoridadMatrixPrivada } from '@sytue/auth/server';
import { bff } from '../../../../../lib/bff.js';
export const GET = rutaAutoridadMatrixPrivada({ bff, env: import.meta.env, product: 'erp' });
export const POST = GET;
