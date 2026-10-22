/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los límites que la interfaz conoce para no mandar lo que el servicio rechazaría. Son
 * espejo de `services/bases/internal/domain/tipos.go` (el servicio manda).
 */
export const MAX_BYTES_CSV = 5 << 20;
export const PAGINA = 100;
export const MAX_OPCIONES_ENLACE = 500;
