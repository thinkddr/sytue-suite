/* SPDX-License-Identifier: Apache-2.0 */
/*
 * Los bloques cifrados del Drive de la bóveda, vistos desde Photos: leer y subir
 * texto cifrado, nada más (ver `../../../pq-vault/[...ruta].ts`). Sin DELETE: el
 * puerto de la galería no borra bloques.
 */
import { rutaBlobsMatrixPrivados } from '@sytue/auth/server';
import { bff } from '../../../../../../../lib/bff.js';
export const prerender = false;
export const GET = rutaBlobsMatrixPrivados({ bff, env: import.meta.env, product: 'drive' });
export const PUT = GET;
