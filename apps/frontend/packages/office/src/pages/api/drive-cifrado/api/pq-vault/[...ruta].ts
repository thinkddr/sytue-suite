/* SPDX-License-Identifier: Apache-2.0 */
/*
 * La bóveda de DRIVE vista desde la ofimática (ADR 0086): con el Drive de la persona
 * en la bóveda, «Guardar en Drive» cifra el fichero del documento en el navegador
 * (`editor/state/driveCifrado.ts`). Es la misma puerta que la de Drive y la de
 * Photos, con el ámbito atado a `drive`, bajo un prefijo propio.
 *
 * Sin PUT: la ofimática solo abre la bóveda que ya existe; crearla, vaciarla o
 * cambiar sus claves es cosa de Drive.
 */
import { rutaBovedaPq } from '@sytue/auth/server';
import { bff } from '../../../../../lib/auth';
export const prerender = false;
export const GET = rutaBovedaPq({ bff, env: import.meta.env, product: 'drive' });
export const POST = GET;
