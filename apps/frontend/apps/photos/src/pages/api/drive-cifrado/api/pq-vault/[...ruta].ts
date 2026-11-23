/* SPDX-License-Identifier: Apache-2.0 */
/*
 * La bóveda de DRIVE vista desde Photos (ADR 0086): con el Drive de la persona en la
 * bóveda, la galería sube los originales cifrando en el navegador
 * (`client/driveCifrado.ts`). Es la misma puerta que la de Drive, con el ámbito
 * atado a `drive`, bajo un prefijo propio para no mezclarla con la bóveda de Photos.
 *
 * Sin PUT: Photos solo abre la bóveda que ya existe; crearla, vaciarla o cambiar
 * sus claves es cosa de Drive.
 */
import { rutaBovedaPq } from '@sytue/auth/server';
import { bff } from '../../../../../lib/bff.js';
export const prerender = false;
export const GET = rutaBovedaPq({ bff, env: import.meta.env, product: 'drive' });
export const POST = GET;
