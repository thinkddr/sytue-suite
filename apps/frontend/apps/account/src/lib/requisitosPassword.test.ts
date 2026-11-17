/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La lista de requisitos tiene que contar como `domain.ValidatePassword`: si da
 * por buena una contraseña que el servidor rechaza, la pantalla miente.
 */
import { describe, expect, test } from 'bun:test';
import { leerPolitica, requisitosPassword } from './requisitosPassword.js';

const cumplidos = (r: ReturnType<typeof requisitosPassword>) =>
  Object.fromEntries(r.map((x) => [x.clave, x.cumplido]));

describe('requisitosPassword', () => {
  test('sin complejidad: longitud y distinta de la actual, en ese orden', () => {
    const r = requisitosPassword({ min_length: 12, require_complexity: false }, '', 'vieja');
    expect(r.map((x) => x.clave)).toEqual(['length', 'different']);
    expect(r.every((x) => !x.cumplido)).toBe(true);
  });

  test('la longitud se cuenta en puntos de código, como las runas de Go', () => {
    const p = { min_length: 8, require_complexity: false };
    // 7 caracteres con un emoji (2 unidades UTF-16): 8 en `.length`, 7 runas.
    expect(cumplidos(requisitosPassword(p, 'abcdef😀', ''))['length']).toBe(false);
    expect(cumplidos(requisitosPassword(p, 'ñandúes8', ''))['length']).toBe(true);
  });

  test('complejidad: una letra de cualquier alfabeto y un dígito ASCII', () => {
    const p = { min_length: 8, require_complexity: true };
    const cx = (s: string) => cumplidos(requisitosPassword(p, s, ''))['complexity'];
    // Los mismos casos que `TestValidatePassword` en domain/secret_test.go.
    expect(cx('solopalabras')).toBe(false);
    expect(cx('123456789')).toBe(false);
    expect(cx('1234!@#$%^')).toBe(false);
    expect(cx('palabra12')).toBe(true);
    expect(cx('Passw0rd')).toBe(true);
    expect(cx('ñandú123')).toBe(true);
    // Un dígito que no es ASCII no cuenta (Go mira '0'..'9').
    expect(cx('palabra٣')).toBe(false);
  });

  test('repetir la actual no vale', () => {
    const p = { min_length: 8, require_complexity: false };
    expect(cumplidos(requisitosPassword(p, 'la-de-siempre', 'la-de-siempre'))['different']).toBe(
      false,
    );
    expect(cumplidos(requisitosPassword(p, 'otra-distinta', 'la-de-siempre'))['different']).toBe(
      true,
    );
  });
});

describe('leerPolitica', () => {
  test('lee la de /me y descarta lo que no encaja', () => {
    expect(leerPolitica({ password_policy: { min_length: 14, require_complexity: true } })).toEqual(
      { min_length: 14, require_complexity: true },
    );
    expect(leerPolitica({ password_policy: { min_length: 10 } })).toEqual({
      min_length: 10,
      require_complexity: false,
    });
    expect(leerPolitica({ mfa_enabled: true })).toBeNull();
    expect(leerPolitica({ password_policy: { min_length: '12' } })).toBeNull();
    expect(leerPolitica(null)).toBeNull();
  });
});
