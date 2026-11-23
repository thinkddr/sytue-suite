/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El Drive de la persona está cifrado de extremo a extremo (la bóveda de la ADR
 * 0086): Drive rechaza escribir con 409 y el código estable
 * `private_storage_migration_required`. El BFF de la galería no puede dejar ahí
 * nada en claro, así que lo dice con su propio 409 (`drive_private_vault`) y la
 * galería sube cifrando en el navegador (`client/driveCifrado.ts`). Cualquier otro
 * 409 sigue siendo un fallo de Drive.
 */

/** El código con el que Drive dice que su fuente está congelada (services/drive). */
export const CODIGO_UNIDAD_CIFRADA = 'private_storage_migration_required';
/** El error con el que `/api/drive-subir` lo cuenta a la galería. */
export const ERROR_DRIVE_CIFRADO = 'drive_private_vault';

/** El `code` de un cuerpo de error de la casa (`{error: {code}}`), o `undefined`. */
export function codigoDeError(cuerpo: string): string | undefined {
  try {
    const datos = JSON.parse(cuerpo) as { error?: { code?: unknown } } | null;
    const code = datos?.error?.code;
    return typeof code === 'string' ? code : undefined;
  } catch {
    return undefined;
  }
}

/** ¿Es el 409 de la unidad cifrada? Solo con ese código: otro 409 no lo es. */
export function esUnidadCifrada(status: number, code: string | undefined): boolean {
  return status === 409 && code === CODIGO_UNIDAD_CIFRADA;
}
