// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import catalog from './office-catalog.json';

export type OfficeExportFormat = 'pdf' | 'ooxml' | 'odf' | 'html' | 'markdown';
export type OfficeKind = 'document' | 'spreadsheet' | 'presentation';

/** Copia de build de apps/nucleo/office/catalog.json; el CI exige igualdad. */
export const OFFICE_EXPORTS = catalog.exports as readonly {
  id: OfficeExportFormat;
  online: boolean;
  labels: Record<OfficeKind, string>;
  /** Interruptor de la descarga (hoy solo «PDF accesible»): una entrada con tic debajo. */
  option?: { id: string; label: string; default: boolean };
}[];
export const OFFICE_EXPORT_FORMATS: readonly OfficeExportFormat[] = OFFICE_EXPORTS.map((e) => e.id);

/** Disponibilidad de UI, NO autorización: el backend comprueba sus ACL. */
export function officeExportAvailable(
  command: (typeof OFFICE_EXPORTS)[number],
  offline: boolean,
  busy: boolean,
): boolean {
  return !busy && !(command.online && offline);
}
