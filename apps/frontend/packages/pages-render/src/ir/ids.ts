/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import type { Id } from './tipos';

const ALFABETO = 'abcdefghijklmnopqrstuvwxyz0123456789';

/** Id nuevo del IR: 10 caracteres de [a-z0-9] (≈ 51 bits). Espejo de `ir.NuevoID` en Go. */
export function nuevoId(): Id {
  const b = crypto.getRandomValues(new Uint8Array(10));
  // ponytail: sesgo de módulo (256 % 36 = 4) despreciable para ids de un documento, no para secretos.
  return Array.from(b, (x) => ALFABETO[x % ALFABETO.length]).join('');
}
