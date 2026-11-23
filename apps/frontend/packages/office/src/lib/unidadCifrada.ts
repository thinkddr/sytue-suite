/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { HttpError } from '@sytue/api-clients';

/**
 * ¿Drive rechazó escribir porque la unidad de esta persona está cifrada de extremo
 * a extremo (la bóveda de la ADR 0086)? Lo dice con 409 y el código estable
 * `private_storage_migration_required` (services/drive). Solo eso: otro 409 es un
 * fallo cualquiera y no se toma por «la unidad está cifrada».
 */
export function esUnidadCifrada(error: unknown): boolean {
  return (
    error instanceof HttpError &&
    error.status === 409 &&
    error.code === 'private_storage_migration_required'
  );
}
