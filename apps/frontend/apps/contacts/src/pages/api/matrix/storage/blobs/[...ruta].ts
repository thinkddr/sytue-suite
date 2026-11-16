/* SPDX-License-Identifier: Apache-2.0 */
import { rutaBlobsMatrixPrivados } from '@sytue/auth/server';
import { bff } from '../../../../../lib/bff.js';
export const prerender = false;
export const GET = rutaBlobsMatrixPrivados({ bff, env: import.meta.env, product: 'contacts' });
export const PUT = GET;
export const DELETE = GET;
