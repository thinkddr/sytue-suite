/* SPDX-License-Identifier: Apache-2.0 */
import { rutaMigracionMatrixPrivada } from '@sytue/auth/server';
import { bff } from '@sytue/office/auth';
export const prerender = false;
export const GET = rutaMigracionMatrixPrivada({ bff, env: import.meta.env, product: 'docs' });
export const POST = GET;
