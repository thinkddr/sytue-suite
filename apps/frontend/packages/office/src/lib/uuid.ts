/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Un id de documento es un UUID. Se exige en el BFF y no solo en el servicio porque
 * `encodeURIComponent` deja pasar `.` y `..`, y `fetch` normaliza esos segmentos:
 * `id: ".."` convertía `/documents/..` en otra ruta del servicio (revisión de
 * seguridad de la papelera, 2026-09-28).
 */
export function esUuid(id: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id);
}
