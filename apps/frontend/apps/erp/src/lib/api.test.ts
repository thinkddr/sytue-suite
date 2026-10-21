/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import { ErrorApi, esSinPermiso, mensajeDeError } from './api.js';

const t = (clave: 'error.generic' | 'error.sinPermiso'): string => `«${clave}»`;

describe('mensajeDeError', () => {
  it('traduce el 403 sin_permiso en vez de pintar el respaldo en inglés', () => {
    const err = new ErrorApi('You do not have access to this module.', 403, {}, 'sin_permiso');
    expect(esSinPermiso(err)).toBe(true);
    expect(mensajeDeError(err, t)).toBe('«error.sinPermiso»');
  });

  it('deja el resto como estaba', () => {
    expect(esSinPermiso(new ErrorApi('x', 403))).toBe(false);
    expect(mensajeDeError(new ErrorApi('El CIF no es válido.', 422), t)).toBe(
      'El CIF no es válido.',
    );
    expect(mensajeDeError('nada', t)).toBe('«error.generic»');
  });
});
