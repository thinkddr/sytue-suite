/* SPDX-License-Identifier: Apache-2.0 */
import { rutaBootstrapMatrixPrivado } from '@sytue/auth/server';
import { bff } from '../../../../lib/bff.js';
export const prerender = false;
export const GET = rutaBootstrapMatrixPrivado({ bff, env: import.meta.env, product: 'erp' });
