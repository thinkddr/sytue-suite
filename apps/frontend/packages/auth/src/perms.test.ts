/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la lectura de permisos granulares del access token (admin delegada).
 * Ejecutar: cd apps/frontend && bun test packages/auth
 */
import { expect, test } from 'bun:test';
import { ALL_PERMISSIONS, permsFromAccessToken } from './perms';

/** Firma un JWT de mentira: solo importa el payload (no se verifica; ver perms.ts). */
function fakeJwt(payload: Record<string, unknown>): string {
  const b64 = (o: unknown): string =>
    btoa(JSON.stringify(o)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64({ alg: 'RS256' })}.${b64(payload)}.firma-irrelevante`;
}

test('lee los permisos del claim perms', () => {
  const t = fakeJwt({ sub: 'u1', perms: ['users.read', 'audit.view'] });
  expect(permsFromAccessToken(t)).toEqual(['users.read', 'audit.view']);
});

test('sin claim perms → vacío (es lo normal en owner/admin: Sytue ID lo omite)', () => {
  expect(permsFromAccessToken(fakeJwt({ sub: 'u1', roles: ['admin'] }))).toEqual([]);
});

test('deny-by-default ante entrada corrupta: nunca lanza, devuelve vacío', () => {
  expect(permsFromAccessToken('')).toEqual([]);
  expect(permsFromAccessToken('no-es-un-jwt')).toEqual([]);
  expect(permsFromAccessToken('a.b')).toEqual([]); // payload no es base64 válido
  expect(permsFromAccessToken(fakeJwt({ perms: 'users.read' }))).toEqual([]); // no es array
  expect(permsFromAccessToken(fakeJwt({ perms: [1, null, {}] }))).toEqual([]);
});

test('descarta permisos DESCONOCIDOS (un backend futuro no abre capacidades por sorpresa)', () => {
  const t = fakeJwt({ perms: ['users.read', 'superpoder.inventado', 'audit.view'] });
  expect(permsFromAccessToken(t)).toEqual(['users.read', 'audit.view']);
});

test('el catálogo espeja el del backend (roles.go): 9 permisos, roles.manage incluido', () => {
  expect(ALL_PERMISSIONS).toHaveLength(9);
  expect(ALL_PERMISSIONS).toContain('roles.manage');
  expect(ALL_PERMISSIONS).toContain('billing.manage');
  // Base64url con relleno: un payload cuya longitud no es múltiplo de 4 debe leerse igual.
  expect(permsFromAccessToken(fakeJwt({ perms: ['sso.manage'], pad: 'xyz' }))).toEqual([
    'sso.manage',
  ]);
});

test('soloOrganizacion cierra ERP, Pages y Bases a las cuentas personales, y nada más', async () => {
  const { soloOrganizacion } = await import('./types.js');
  for (const key of ['erp', 'pages', 'bases']) {
    expect([key, soloOrganizacion(key, { orgKind: 'org' })]).toEqual([key, false]);
    expect([key, soloOrganizacion(key, { orgKind: undefined })]).toEqual([key, false]);
    expect([key, soloOrganizacion(key, null)]).toEqual([key, false]);
    expect([key, soloOrganizacion(key, { orgKind: 'personal' })]).toEqual([key, true]);
    expect([key, soloOrganizacion(key, { orgKind: undefined, plan: 'personal_free' })]).toEqual([
      key,
      true,
    ]);
    expect([key, soloOrganizacion(key, { orgKind: undefined, tenant: 'p-abcdef12' })]).toEqual([
      key,
      true,
    ]);
  }
  // Lo que no es solo de organizaciones se abre también a una cuenta personal.
  expect(soloOrganizacion('mail', { orgKind: 'personal' })).toBe(false);
});
