/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Quién ve qué en Sytue Admin. Lo delicado es la cuenta personal: es `owner`
 * de su propia organización, así que por rol vería TODO. La clase de la
 * organización tiene que mandar sobre el rol, o el alta gratuita abre la
 * consola de administración a cualquiera que se registre.
 */
import { expect, test } from 'bun:test';
import { allowedSections, SECTION_PERMS } from './sections.js';

test('owner y admin ven todas las secciones de una organización', () => {
  expect(allowedSections(['owner'], [])).toEqual(Object.keys(SECTION_PERMS));
  expect(allowedSections(['admin'], [], false, 'org')).toEqual(Object.keys(SECTION_PERMS));
});

test('una cuenta personal DE PAGO ve solo apps y migraciones, aunque sea owner', () => {
  expect(allowedSections(['owner'], [], false, 'personal', 'pro')).toEqual(['apps', 'migrations']);
  expect(
    allowedSections(['owner', 'admin'], ['users.manage'], false, 'personal', 'enterprise'),
  ).toEqual(['apps', 'migrations']);
});

// «Sytue Admin solo con suscripción», que es lo que promete el sitio público: la
// cuenta gratuita no ve NADA de la consola, aunque sea owner. Sin plan en el
// claim tampoco: lo que falta no abre puertas.
test('una cuenta personal GRATUITA no ve ninguna sección', () => {
  expect(allowedSections(['owner'], [], false, 'personal', 'personal_free')).toEqual([]);
  expect(allowedSections(['owner'], [], false, 'personal', undefined)).toEqual([]);
  expect(allowedSections(['owner', 'admin'], ['users.manage'], false, 'personal', '')).toEqual([]);
});

test('sin el claim de clase se trata como organización', () => {
  expect(allowedSections(['owner'], [], false, undefined)).toEqual(Object.keys(SECTION_PERMS));
});

test('un miembro con permisos delegados ve solo lo que sus permisos abren', () => {
  const visibles = allowedSections(['member'], ['audit.view']);
  expect(visibles).toEqual(['audit']);
});
