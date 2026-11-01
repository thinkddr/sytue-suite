/* SPDX-License-Identifier: Apache-2.0 */
import { crearRutaBlobsCifrados, resolveDriveServiceUrl, serverEnv } from '@sytue/auth/server';
import { bff } from '../../../lib/bff.js';

export const prerender = false;
export const { GET, PUT, DELETE } = crearRutaBlobsCifrados({
  bff,
  destinoDrive: () => resolveDriveServiceUrl(serverEnv(import.meta.env)),
  productoFijo: 'rooms',
});
