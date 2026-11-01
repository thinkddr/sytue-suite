/* SPDX-License-Identifier: Apache-2.0 */
import { crearRutaBlobsCifrados } from '@sytue/auth/server';
import { bff, driveBaseUrl } from '../../../lib/auth';

export const prerender = false;
export const { GET, PUT, DELETE } = crearRutaBlobsCifrados({
  bff,
  destinoDrive: driveBaseUrl,
  productoFijo: 'drive',
});
