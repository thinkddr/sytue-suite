/* SPDX-License-Identifier: Apache-2.0 */
import { rutaBootstrapMatrixPrivado } from '@sytue/auth/server';
import { bff } from '@sytue/office/auth';
export const prerender = false;
export const GET = rutaBootstrapMatrixPrivado({ bff, env: import.meta.env, product: 'pdf' });
