/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * @sytue/ai — lo que comparten todas las superficies del asistente.
 *
 * Aquí NO hay DOM ni llamadas: solo el contrato con el BFF y el analizador del
 * Markdown que devuelve el modelo. Cada app pinta los bloques con sus propios
 * comandos, que es lo que impide que la respuesta de un tercero acabe siendo
 * HTML inyectado en once sitios distintos.
 */
export { bloquesDe, trozosDe, textoPlano } from './markdown.js';
export type { Bloque, TipoBloque, Trozo } from './markdown.js';
export { objetoDe, cadena, fechaHora, lista } from './json.js';
