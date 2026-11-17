/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mapeo de una versión del historial de un fichero tal y como la emite el
 * servicio Go (snake_case) a camelCase para la UI. Vive en `@sytue/api-clients`
 * porque CertiPDF lee el mismo historial; aquí solo se le da el nombre de siempre.
 */
export { toFileVersion as toVersion } from '@sytue/api-clients';
export type { FileVersion, RawVersion } from '@sytue/api-clients';
