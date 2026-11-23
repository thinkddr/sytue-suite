/* SPDX-License-Identifier: Apache-2.0 */
import { rutaBovedaPq } from '@sytue/auth/server';
import { bff } from '../../../lib/auth.js';
export const prerender = false;
export const GET = rutaBovedaPq({ bff, env: import.meta.env, product: 'hub' });
export const PUT = GET;
export const POST = GET;
