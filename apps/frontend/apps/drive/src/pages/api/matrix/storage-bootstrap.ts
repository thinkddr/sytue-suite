/* SPDX-License-Identifier: Apache-2.0 */
import {
  crearRutaMatrixStorageBootstrap,
  resolveRoomsServiceUrl,
  serverEnv,
} from '@sytue/auth/server';
import { bff } from '../../../lib/auth';

export const prerender = false;
export const { GET } = crearRutaMatrixStorageBootstrap({
  bff,
  destinoRooms: () => resolveRoomsServiceUrl(serverEnv(import.meta.env)),
  productoFijo: 'drive',
});
