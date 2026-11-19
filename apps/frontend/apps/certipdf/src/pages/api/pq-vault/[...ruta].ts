/* SPDX-License-Identifier: Apache-2.0 */
import { rutaBovedaPq } from '@sytue/auth/server';
import { bff } from '@sytue/office/auth';
export const prerender = false;
export const GET = rutaBovedaPq({ bff, env: import.meta.env, product: 'pdf' });
export const PUT = GET;
export const POST = GET;
